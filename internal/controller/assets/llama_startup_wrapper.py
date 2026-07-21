#!/usr/bin/env python3
# Workaround for asyncpg event loop bug (llama-stack #5978).
# Monkey-patches StackApp to reset SQL engines after temp event loop init.
# Remove when the container image includes the upstream fix (PR #5837).

import gc
import sys

import llama_stack.core.server.server as server_mod
from llama_stack.core.storage.sqlstore.sqlalchemy_sqlstore import SqlAlchemySqlStoreImpl
from sqlalchemy.ext.asyncio import async_sessionmaker

_orig_init = server_mod.StackApp.__init__


class SessionMaker:
    """Recreates the SQLAlchemy engine and session on the current event loop."""

    def __init__(self, store):
        self._store = store
        self._maker = None

    def __call__(self):
        if self._maker is None:
            self._store._engine = self._store.create_engine()
            self._maker = async_sessionmaker(self._store._engine)
            self._store.async_session = self._maker
        return self._maker()


def _patched_init(self, config, *args, **kwargs):
    _orig_init(self, config, *args, **kwargs)
    for obj in gc.get_objects():
        if isinstance(obj, SqlAlchemySqlStoreImpl):
            obj._engine = None
            obj.async_session = SessionMaker(obj)


server_mod.StackApp.__init__ = _patched_init

from llama_stack.cli.llama import main  # noqa: E402

sys.argv[0] = "llama"
main()
