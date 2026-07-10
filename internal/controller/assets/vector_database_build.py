#!/usr/bin/env python3

#
# Copyright 2026.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Vector database configuration builder for the OpenStack Lightspeed operator.

Runs as the second init container (`vector-database-config-build`), after
`vector_database_collect.sh`. It loads operator-provided base configs, walks every
vector DB directory left by the collect step, and writes merged configs back to
the shared volume.

Input layout (under --vector-db-path, produced by vector_database_collect.sh):
  {vector-db-path}/
  └── <image_uuid_dir>/             (random directory name from collect script)
      ├── vector_db/
      │   ├── <vector-db-name>/
      │   │   ├── llama-stack.yaml
      │   │   └── faiss_store.db
      └── embeddings_model/

Output (written to --vector-db-path, same basenames as the base configs):
  {vector-db-path}/
  ├── ogx_config.yaml
  ├── lightspeed-stack.yaml
  └── <collect-dir>/                (collected data preserved)

Processing:
  1. For each subdirectory of */vector_db/, read its llama-stack.yaml.
  2. For each detected llama-stack.yaml file, extract its data and inject
     the relevant entries into the output ogx_config.yaml and lightspeed-stack.yaml
     files.
  3. Write the merged YAML next to the collected data.

Warning: This script only injects values into existing config structures.
Base configs MUST contain otherwise this script will fail:
- OGX config: registered_resources.{models,vector_stores}, storage.backends,
              providers.{inference,vector_io}
- Lightspeed Stack config: byok_rag, rag.inline

Arguments:
  --vector-db-path            Shared volume path (input collected data, output configs)
  --ogx-config-path           Path to the base OGX configuration file
  --lightspeed-stack-path     Path to the base Lightspeed Stack configuration file
"""

import argparse
import functools
from pathlib import Path
from typing import Any, Iterable, Optional, Callable
import logging
import sys

import yaml

# Template for the directory path where data for a single vector database
# instance resides. In the configuration, VECTOR_DB_DATA_PATH will typically
# be substituted by the operator as an environment variable.
VECTOR_DB_DIR_TEMPLATE = (
    "${{env.VECTOR_DB_DATA_PATH}}/{uuid}/vector_db/{vector_db_name}"
)

# Template for the directory path where data for an embedding model resides. In
# the configuration, VECTOR_DB_DATA_PATH will be substituted by OGX using
# an environment variable.
EMBEDDING_MODEL_DIR_TEMPLATE = "${{env.VECTOR_DB_DATA_PATH}}/{uuid}/embeddings_model"

# Template for a file path where vector db data are stored.
VECTOR_DB_DATA_PATH_TEMPLATE = f"{VECTOR_DB_DIR_TEMPLATE}/faiss_store.db"

# The original configuration file name for OGX in the mounted vector database data.
# Update later: The file is still named 'llama-stack.yaml' for backward compatibility,
# since the Llama Stack project was renamed to OGX recently.
OGX_CONFIG_SOURCE_FILE_NAME = "llama-stack.yaml"


# -- Shared functions --------------------------------------------------------
def load_yaml_file(yaml_file_path: Path) -> dict[str, Any]:
    """Load YAML file"""
    try:
        with open(yaml_file_path, "r", encoding="utf-8") as f:
            return yaml.safe_load(f) or {}
    except FileNotFoundError:
        logging.error("YAML file not found: %s", yaml_file_path)
        sys.exit(1)


def add_unique(lst: list, item: Any, key: Optional[str] = None) -> None:
    """Add item to list if not already present.

    :param lst:  List to modify in-place
    :param item: Item to add
    :param key:  If provided, check uniqueness by comparing item[key] values.
                 If None, check direct item equality.
    """
    if key:
        if any(existing.get(key) == item.get(key) for existing in lst):
            return
    elif item in lst:
        return
    lst.append(item)


def write_yaml_file(yaml_data: dict[str, Any], dest_path: Path) -> None:
    """Write YAML data to the specified file path."""
    try:
        dest_path.parent.mkdir(parents=True, exist_ok=True)
        with open(dest_path, "w", encoding="utf-8") as f:
            yaml.dump(yaml_data, f, default_flow_style=False, sort_keys=False)
    except (OSError, yaml.YAMLError) as e:
        logging.error("Failed to write YAML to %s: %s", dest_path, e)
        sys.exit(1)


def iterate_vector_db_data_dir(vector_db_data_dir_path: Path) -> Iterable[Path]:
    """Return all folders inside any vector_db/ subfolder, one per yield."""
    for image_uuid_dir in vector_db_data_dir_path.iterdir():
        vector_db_path = image_uuid_dir.joinpath("vector_db")

        if not vector_db_path.is_dir():
            continue

        for folder in vector_db_path.iterdir():
            if folder.is_dir():
                yield folder


def config_build(
    vector_db_parent_dir: Path,
    config_target_path: Path,
    config_populate_fn: Callable[[Path, dict[str, Any]], dict[str, Any]],
) -> None:
    config_target = load_yaml_file(config_target_path)
    for vector_db_dir in iterate_vector_db_data_dir(vector_db_parent_dir):
        ogx_config_source_path = vector_db_dir.joinpath(OGX_CONFIG_SOURCE_FILE_NAME)
        try:
            config_target = config_populate_fn(ogx_config_source_path, config_target)
        except (KeyError, IndexError) as e:
            logging.error(
                "Error processing config: missing required section in source "
                "or target file (%s)",
                e,
            )
            sys.exit(1)

    config_product_path = vector_db_parent_dir.joinpath(config_target_path.name)
    write_yaml_file(config_target, config_product_path)


# ----------------------------------------------------------------------------


# -- OGX functions -----------------------------------------------------------
def ogx_process(ogx_config_source_path: Path, ogx_config_target: dict[str, Any]):
    """Populate the target OGX config with vector DB data from source OGX config"""
    ogx_config_source = load_yaml_file(ogx_config_source_path)

    # E.g.: /data/<uuid>/vector_db/os_product_docs/llama-stack.yaml -> <uuid>
    image_uuid = ogx_config_source_path.parts[-4]

    # E.g.: /data/<uuid>/vector_db/os_product_docs/llama-stack.yaml -> os_product_docs
    vector_db_name = ogx_config_source_path.parts[-2]

    vector_db_file = VECTOR_DB_DATA_PATH_TEMPLATE.format(
        uuid=image_uuid, vector_db_name=vector_db_name
    )
    embedding_model_dir = EMBEDDING_MODEL_DIR_TEMPLATE.format(uuid=image_uuid)

    # Populate registered_resources.models
    src_model = ogx_config_source["registered_resources"]["models"][0].copy()
    src_model["provider_model_id"] = embedding_model_dir
    tgt_models = ogx_config_target["registered_resources"]["models"]
    add_unique(tgt_models, src_model, "model_id")

    # Populate registered_resources.vector_stores
    embedding_model = f"{src_model['provider_id']}/{embedding_model_dir}"
    src_vstore = ogx_config_source["registered_resources"]["vector_stores"][0].copy()
    src_vstore["embedding_model"] = embedding_model
    tgt_vstores = ogx_config_target["registered_resources"]["vector_stores"]
    add_unique(tgt_vstores, src_vstore)

    # Populate storage.backends
    storage_backend_key = f"kv_rag_{image_uuid}_{vector_db_name}"
    storage_backend = ogx_config_source["storage"]["backends"]["kv_rag"].copy()
    storage_backend["db_path"] = vector_db_file
    ogx_config_target["storage"]["backends"][storage_backend_key] = storage_backend

    # Populate providers.inference
    src_inference = ogx_config_source["providers"]["inference"][0]
    tgt_inferences = ogx_config_target["providers"]["inference"]
    add_unique(tgt_inferences, src_inference)

    # Populate providers.vector_io
    src_vector_io = ogx_config_source["providers"]["vector_io"][0].copy()
    src_vector_io["config"]["persistence"]["backend"] = storage_backend_key
    tgt_vector_ios = ogx_config_target["providers"]["vector_io"]
    add_unique(tgt_vector_ios, src_vector_io)

    return ogx_config_target


# ----------------------------------------------------------------------------


# -- Lightspeed Stack functions ----------------------------------------------
def lstack_process(
    ogx_config_source_path: Path,
    lstack_config_target: dict[str, Any],
    disable_rag_entries: bool = False,
) -> dict[str, Any]:
    """Update Lightspeed stack config with RAG entries from OGX config source."""
    ogx_config_source = load_yaml_file(ogx_config_source_path)

    src_vstores = ogx_config_source["registered_resources"]["vector_stores"]
    vector_store_id = src_vstores[0]["vector_store_id"]

    add_unique(
        lstack_config_target["byok_rag"],
        {
            "rag_id": vector_store_id,
            "vector_db_id": vector_store_id,
            # The score multiplier is set to 1.0 so all BYOK sources have
            # equal weighting.
            "score_multiplier": 1.0,
            # The Lightspeed Stack currently requires a "db_path" value even
            # when OGX operates in server mode. This placeholder value ("NONE")
            # is provided solely to satisfy this requirement and should be
            # removed once the Lightspeed Stack no longer mandates it for
            # server mode.
            "db_path": "NONE",
        },
    )

    if not disable_rag_entries:
        add_unique(lstack_config_target["rag"]["inline"], vector_store_id)
    return lstack_config_target


# ----------------------------------------------------------------------------


def parse_arguments() -> argparse.Namespace:
    """Parse command-line arguments and return parsed namespace."""
    parser = argparse.ArgumentParser(
        description=(
            "Build vector database configuration files by merging collected "
            "vector DB data with base configs"
        )
    )
    parser.add_argument(
        "--vector-db-path",
        type=Path,
        required=True,
        help="Path (as pathlib.Path) to the mounted vector DB data volume and output destination",
    )
    parser.add_argument(
        "--ogx-config-path",
        type=Path,
        required=True,
        help="Path (as pathlib.Path) to the base OGX configuration file",
    )
    parser.add_argument(
        "--lightspeed-stack-path",
        type=Path,
        required=True,
        help="Path (as pathlib.Path) to the base Lightspeed Stack configuration file",
    )
    parser.add_argument(
        "--disable-rag-entries",
        action="store_true",
        default=False,
        help="When set, skip adding vector store IDs to rag.inline",
    )

    return parser.parse_args()


def main() -> None:
    """main"""
    args = parse_arguments()
    config_build(args.vector_db_path, args.ogx_config_path, ogx_process)
    lstack_fn = functools.partial(
        lstack_process, disable_rag_entries=args.disable_rag_entries
    )
    config_build(args.vector_db_path, args.lightspeed_stack_path, lstack_fn)


if __name__ == "__main__":
    main()
