import os
import sqlite3

import mysql.connector
from flask import Flask
from sqlalchemy.pool import QueuePool

app = Flask(__name__)

mysql_connection_env = {
    "host": os.getenv("ISUCON_DB_HOST", "127.0.0.1"),
    "port": os.getenv("ISUCON_DB_PORT", 3306),
    "user": os.getenv("ISUCON_DB_USER", "isucon"),
    "password": os.getenv("ISUCON_DB_PASSWORD", "isucon"),
    "database": os.getenv("ISUCON_DB_NAME", "isuports"),
}

cnxpool = QueuePool(
    lambda: mysql.connector.connect(**mysql_connection_env), pool_size=10
)


def connect_admin_db():
    """管理用DBに接続する"""
    return cnxpool.connect()


def tenant_db_path(id: int) -> str:
    """テナントDBのパスを返す"""
    tenant_db_dir = os.getenv("ISUCON_TENANT_DB_DIR", "../tenant_db")
    return tenant_db_dir + f"/{id}.db"


def connect_to_tenant_db(id: int):
    """テナントDBに接続する"""
    path = tenant_db_path(id)
    return sqlite3.connect(path)


if __name__ == "__main__":
    app.run(port=3000, debug=True, threaded=True)
