#!/usr/bin/env python3
"""Pre-commit-only check enforcing collectors on every KUTTL assert step."""

import sys
from pathlib import Path
import yaml


def validate_kuttl_asserts():
    missing_collectors = 0
    assert_files = sorted(Path("test/kuttl").rglob("*assert*.yaml"))
    if not assert_files:
        print("No KUTTL assert files found.")
        sys.exit(0)

    for file_path in assert_files:
        try:
            with open(file_path, "r", encoding="utf-8") as f:
                documents = list(yaml.safe_load_all(f))
        except Exception as e:
            print(f"❌ Error parsing {file_path}: {e}")
            missing_collectors = 1
            continue

        test_asserts = [
            doc
            for doc in documents
            if isinstance(doc, dict) and doc.get("kind") == "TestAssert"
        ]

        if not test_asserts:
            print(
                f"❌ Enforce Error: {file_path} is missing a TestAssert "
                "with collector set (see other test cases)."
            )
            missing_collectors = 1
            continue

        for test_assert in test_asserts:
            collectors = test_assert.get("collectors")
            if not collectors:
                print(
                    f"❌ Enforce Error: {file_path} has a TestAssert "
                    "without a 'collectors' section."
                )
                missing_collectors = 1
                break

    if missing_collectors != 0:
        print(
            """
            HINT: Ensure that each *-assert.yaml file contains at least:
            ---
            apiVersion: kuttl.dev/v1beta1
            kind: TestAssert
            collectors:
              - type: command
                command: ../../common/collectors/collect-workload-diagnostics.sh
            """
        )

        sys.exit(1)

    print("✅ All TestAssert files contain collectors.")


if __name__ == "__main__":
    validate_kuttl_asserts()
