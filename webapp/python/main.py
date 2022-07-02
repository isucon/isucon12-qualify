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

cnxpool = QueuePool(lambda: mysql.connector.connect(**mysql_connection_env), pool_size=10)


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


@app.route("/api/admin/tenants/add", methods=["POST"])
def admin_add_tenants():
    """
    SasS管理者用API
    テナントを追加する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/admin/tenants/billing", methods=["GET"])
def admin_get_tenants_billing():
    """
    SaaS管理者用API
    テナントごとの課金レポートを最大20件、テナントのid降順で取得する
    URL引数beforeを指定した場合、指定した値よりもidが小さいテナントの課金レポートを取得する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/organizer/players", methods=["GET"])
def organizer_get_players():
    """
    テナント管理者向けAPI
    参加者一覧を返す
    """
    raise NotImplementedError()  # TODO


@app.route("/api/organizer/players/add", methods=["POST"])
def organizer_add_players():
    """
    テナント管理者向けAPI
    テナントに参加者を追加する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/organizer/player/<player_id>/disqualified", methods=["POST"])
def organizer_disqualified_players(player_id: str):
    """
    テナント管理者向けAPI
    参加者を失格にする
    """
    raise NotImplementedError()  # TODO


@app.route("/api/organizer/competitions/add", methods=["POST"])
def organizer_add_competitions():
    """
    テナント管理者向けAPI
    大会を追加する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/organizer/competition/<competition_id>/finish", methods=["POST"])
def organizer_finish_competitions(competition_id: str):
    """
    テナント管理者向けAPI
    大会を終了する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/organizer/competition/<competition_id>/score", methods=["POST"])
def organizer_score_competitions(competition_id: str):
    """
    テナント管理者向けAPI
    大会のスコアをCSVでアップロードする
    """
    raise NotImplementedError()  # TODO


@app.route("/api/organizer/billing", methods=["GET"])
def organizer_get_billing():
    """
    テナント管理者向けAPI
    テナント内の課金レポートを取得する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/organizer/competitions", methods=["GET"])
def organizer_get_competitions():
    """
    主催者向けAPI
    大会の一覧を取得する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/player/player/<player_id>", methods=["GET"])
def player_get_detail(player_id: str):
    """
    参加者向けAPI
    参加者の詳細情報を取得する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/player/competition/<competition_id>/ranking", methods=["GET"])
def player_get_competition_ranking(competition_id):
    """
    参加者向けAPI
    大会ごとのランキングを取得する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/player/competitions", methods=["GET"])
def player_get_competitions(estate_id):
    """
    参加者向けAPI
    大会の一覧を取得する
    """
    raise NotImplementedError()  # TODO


@app.route("/api/me", methods=["GET"])
def get_me():
    """
    共通API
    JWTで認証した結果、テナントやユーザ情報を返す
    """
    raise NotImplementedError()  # TODO


@app.route("/initialize", methods=["POST"])
def bench_initialize():
    """
    ベンチマーカー向けAPI
    ベンチマーカーが起動したときに最初に呼ぶ
    データベースの初期化などが実行されるため、スキーマを変更した場合などは適宜改変すること
    """
    raise NotImplementedError()  # TODO


if __name__ == "__main__":
    app.run(port=3000, debug=True, threaded=True)
