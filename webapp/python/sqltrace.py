import json
import os
from datetime import datetime

from sqlalchemy.engine import Engine


def initialize_sql_logger(engine: Engine) -> Engine:

    trace_file_path = os.getenv("ISUCON_SQLITE_TRACE_FILE")
    if not trace_file_path:
        return engine

    def execute_with_trace(statement, *multiparams, **params):
        start_time = datetime.now()
        start = start_time.timestamp()

        res = engine.execute_org(statement, *multiparams, **params)

        end = datetime.now().timestamp()
        query_time = end - start

        affected_rows = 0
        if res.rowcount > 0:
            affected_rows = res.rowcount

        sql_trace_log = {
            "time": start_time.isoformat(),
            "statement": statement,
            "args": [arg for arg in multiparams],
            "query_time": query_time,
            "affected_rows": affected_rows,
        }

        with open(trace_file_path, "a") as f:
            f.write(json.dumps(sql_trace_log) + "\n")
        return res

    engine.execute_org = engine.execute
    engine.execute = execute_with_trace
    return engine
