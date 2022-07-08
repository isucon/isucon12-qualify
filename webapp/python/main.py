import codecs
import csv
import fcntl
import io
import os
import re
import sqlite3
import subprocess
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Optional

import jwt
import mysql.connector
from flask import Flask, abort, jsonify, make_response, request
from sqlalchemy.pool import QueuePool
from werkzeug.exceptions import HTTPException

INITIALIZE_SCRIPT = "../sql/init.sh"
COOKIE_NAME = "isuports_session"
TENANT_DB_SCHEMA_FILE_PATH = "../sql/tenant/10_schema.sql"

ROLE_ADMIN = "admin"
ROLE_ORGANIZER = "organizer"
ROLE_PLAYER = "player"
ROLE_NONE = "none"

# 正しいテナント名の正規表現
TENANT_NAME_REGEXP = re.compile(r"^[a-z][a-z0-9-]{0,61}[a-z0-9]$")

app = Flask(__name__)

mysql_connection_env = {
    "host": os.getenv("ISUCON_DB_HOST", "127.0.0.1"),
    "port": os.getenv("ISUCON_DB_PORT", 3306),
    "user": os.getenv("ISUCON_DB_USER", "isucon"),
    "password": os.getenv("ISUCON_DB_PASSWORD", "isucon"),
    "database": os.getenv("ISUCON_DB_NAME", "isuports"),
}

cnxpool = QueuePool(lambda: mysql.connector.connect(**mysql_connection_env), pool_size=10)


def select_all(cnx, query, *args, dictionary=True):
    # 管理用DBに接続する
    try:
        cur = cnx.cursor(dictionary=dictionary)
        cur.execute(query, *args)
        return cur.fetchall()
    finally:
        cnx.close()


def select_row(cnx, *args, **kwargs):
    rows = select_all(cnx, *args, **kwargs)
    return rows[0] if len(rows) > 0 else None


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


def create_tenant_db(id: int):
    """テナントDBを新規に作成する"""
    path = tenant_db_path(id)

    command = f"sqlite3 {path} < {TENANT_DB_SCHEMA_FILE_PATH}"
    subprocess.run(["bash", "-c", command])


def dispense_id() -> str:
    """システム全体で一意なIDを生成する"""
    id = 0
    last_err = None
    for i in range(100):
        cnx = connect_admin_db()
        try:
            cur = cnx.cursor()
            cur.execute("REPLACE INTO id_generator (stub) VALUES (%s)", ("a",))
            cnx.commit()
        except mysql.connector.Error as e:
            if e.errno == 1213:  # deadlock
                last_err = e
                continue
            cnx.rollback()
            raise e
        finally:
            id = cur.lastrowid
            cnx.close()
    if id != 0:
        return str(id)
    raise last_err


@app.errorhandler(HTTPException)
def error_handler(e):
    return make_response(e.description, e.code, {"Content-Type": "text/plain"})


@dataclass
class SuccessResult:
    status: bool
    data: Any


@dataclass
class FailureResult:
    status: bool
    message: str


@dataclass
class Viewer:
    """アクセスしたきた人の情報"""

    role: str
    player_id: str
    tenant_name: str
    tenant_id: int


def parse_viewer() -> Viewer:
    """リクエストヘッダをパースしてViewerを返す"""
    token_str = request.cookies.get(COOKIE_NAME)
    if not token_str:
        abort(401, f"cookie {COOKIE_NAME} is not found")

    key_filename = os.getenv("ISUCON_JWT_KEY_FILE", "../go/public.pem")
    key = open(key_filename, "r").read()

    tenant = retrieve_tenant_row_from_header()
    token = jwt.decode(token_str, key, audience=tenant.name, algorithms=["RS256"])
    if not token.get("sub"):
        abort(401, f"invalid token: subject is not found in token: {token_str}")

    role = token.get("role")
    if not role:
        abort(401, f"invalid token: role is not found: {token_str}")

    if role not in [ROLE_ADMIN, ROLE_ORGANIZER, ROLE_PLAYER]:
        abort(401, f"invalid token: role is not found: {token_str}")

    aud = token.get("aud")
    if len(aud) != 1:
        abort(401, f"invalid token: aud field is few or too much: {token_str}")

    if tenant.name == "admin" and role != ROLE_ADMIN:
        abort(401, "tenant not found")

    if tenant.name != aud[0]:
        abort(401, f"invalid token: tenant name is not match with {request.host}: {token_str}")

    return Viewer(
        role=role,
        player_id=token.get("sub"),
        tenant_name=tenant.name,
        tenant_id=tenant.id,
    )


def retrieve_tenant_row_from_header():
    """JWTに入っているテナント名とHostヘッダのテナント名が一致しているか確認"""
    base_host = os.getenv("ISUCON_BASE_HOSTNAME", ".t.isucon.dev")
    tenant_name = request.host.removesuffix(base_host)

    # SaaS管理者用ドメイン
    if tenant_name == "admin":
        return TenantRow(
            name="admin",
            display_name="admin",
        )

    # テナントの存在確認
    cnx = connect_admin_db()
    row = None
    try:
        query = "SELECT * FROM tenant WHERE name = (%s)"
        row = select_row(cnx, query, (tenant_name,))
    finally:
        cnx.close()

    if row is None:
        abort(401, "tenant not found")

    return TenantRow(**row)


@dataclass
class TenantRow:
    name: str
    display_name: str
    id: Optional[int] = None
    created_at: Optional[int] = None
    updated_at: Optional[int] = None


@dataclass
class PlayerRow:
    tenant_id: int
    id: str
    display_name: str
    is_disqualified: bool
    created_at: int
    updated_at: int


def retrieve_player(tenant_db, id: str) -> PlayerRow:
    """参加者を取得する"""
    tenant_db.row_factory = sqlite3.Row
    query = "SELECT * FROM player WHERE id = (?)"
    cur = tenant_db.cursor()
    cur.execute(query, (id,))
    row = cur.fetchone()
    if not row:
        return None

    return PlayerRow(
        tenant_id=row["tenant_id"],
        id=row["id"],
        display_name=row["display_name"],
        is_disqualified=bool(row["is_disqualified"]),
        created_at=row["created_at"],
        updated_at=row["updated_at"],
    )


@dataclass
class CompetitionRow:
    tenant_id: int
    id: str
    title: str
    finished_at: Optional[int]
    created_at: int
    updated_at: int


def retrieve_competition(tenant_db, id: str) -> Optional[CompetitionRow]:
    """大会を取得する"""
    tenant_db.row_factory = sqlite3.Row
    query = "SELECT * FROM competition WHERE id = ?"
    cur = tenant_db.cursor()
    cur.execute(query, (id,))
    row = cur.fetchone()
    if not row:
        return None

    return CompetitionRow(**row)


@dataclass
class PlayerScoreRow:
    tenant_id: int
    id: str
    player_id: str
    competition_id: str
    score: int
    row_num: int
    created_at: int
    updated_at: int


def lock_file_path(id: int) -> str:
    """排他ロックのためのファイル名を生成する"""
    tenant_db_dir = os.getenv("ISUCON_TENANT_DB_DIR", "../tenant_db")
    return os.path.join(tenant_db_dir, f"{id}.lock")


def flock_by_tenant_id(tenant_id):
    """排他ロックする"""
    path = lock_file_path(tenant_id)
    fd = os.open(path, os.O_RDWR | os.O_CREAT | os.O_TRUNC)
    lock_file_fd = None
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except (IOError, OSError):
        pass
    else:
        lock_file_fd = fd
    return lock_file_fd


@dataclass
class TenantDetail:
    name: str
    display_name: str


@app.route("/api/admin/tenants/add", methods=["POST"])
def admin_add_tenants():
    """
    SasS管理者用API
    テナントを追加する
    """
    viewer = parse_viewer()
    if viewer.tenant_name != "admin":
        # admin: SaaS管理者用の特別なテナント名
        abort(404, f"{viewer.tenant_name} has not this API")

    if viewer.role != "admin":
        abort(403, "admin role required")

    display_name = request.values.get("display_name")
    name = request.values.get("name")

    validate_tenant_name(name)

    now = int(datetime.now().timestamp())
    cnx = connect_admin_db()
    try:
        cur = cnx.cursor()
        cur.execute(
            "INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (%s, %s, %s, %s)",
            (name, display_name, now, now),
        )
        cnx.commit()
    except mysql.connector.Error as e:
        if e.errno == 1062:  # duplicate entry
            abort(400, "duplicate tenant")
        cnx.rollback()
        raise e
    finally:
        id = cur.lastrowid

        cnx.close()

    create_tenant_db(id)

    return jsonify(SuccessResult(status=True, data={"tenant": TenantDetail(name, display_name)}))


def validate_tenant_name(name):
    """テナント名が規則に沿っているかチェックする"""
    if TENANT_NAME_REGEXP.match(name) is None:
        abort(400, f"invalid tenant name: {name}")


@dataclass
class BillingReport:
    competition_id: str
    competition_title: str
    player_count: int  # スコアを登録した参加者数
    visitor_count: int  # ランキングを閲覧だけした(スコアを登録していない)参加者数
    billing_player_yen: int  # 請求金額 スコアを登録した参加者分
    billing_visitor_yen: int  # 請求金額 ランキングを閲覧だけした(スコアを登録していない)参加者分
    billing_yen: int  # 合計請求金額


@dataclass
class VisitHistoryRow:
    player_id: str
    tenant_id: int
    competition_id: str
    created_at: int
    updated_at: int


@dataclass
class VisitHistorySummaryRow:
    player_id: str
    min_created_at: int


@dataclass
class TenantWithBilling:
    id: str
    name: str
    display_name: str
    billing: int


@dataclass
class PlayerDetail:
    id: str
    display_name: str
    is_disqualified: bool


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
    viewer = parse_viewer()
    if viewer.role != ROLE_ORGANIZER:
        abort(403, "role organizer required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    tenant_db.row_factory = sqlite3.Row
    cur = tenant_db.cursor()
    cur.execute(
        "SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC",
        (viewer.tenant_id,),
    )
    tenant_db.commit()
    rows = cur.fetchall()

    player_details = []
    for row in rows:
        player_details.append(
            PlayerDetail(
                id=row["id"],
                display_name=row["display_name"],
                is_disqualified=bool(row["is_disqualified"]),
            )
        )

    return jsonify(SuccessResult(status=True, data={"players": player_details}))


@app.route("/api/organizer/players/add", methods=["POST"])
def organizer_add_players():
    """
    テナント管理者向けAPI
    テナントに参加者を追加する
    """
    viewer = parse_viewer()
    if viewer.role != ROLE_ORGANIZER:
        abort(403, "role organizer required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    display_names = request.values.getlist("display_name[]")

    player_details = []
    for display_name in display_names:
        id = dispense_id()

        now = int(datetime.now().timestamp())

        cur = tenant_db.cursor()
        cur.execute(
            "INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
            (id, viewer.tenant_id, display_name, False, now, now),
        )
        tenant_db.commit()

        player = retrieve_player(tenant_db, id)
        player_details.append(
            PlayerDetail(
                id=player.id,
                display_name=player.display_name,
                is_disqualified=player.is_disqualified,
            )
        )

    tenant_db.close()

    return jsonify(SuccessResult(status=True, data={"players": player_details}))


@app.route("/api/organizer/player/<player_id>/disqualified", methods=["POST"])
def organizer_disqualified_players(player_id: str):
    """
    テナント管理者向けAPI
    参加者を失格にする
    """
    viewer = parse_viewer()
    if viewer.role != ROLE_ORGANIZER:
        abort(403, "role organizer required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    now = int(datetime.now().timestamp())
    id = dispense_id()

    cur = tenant_db.cursor()
    cur.execute(
        "UPDATE player SET is_disqualified = ?, updated_at = ? WHERE id = ?",
        (True, now, player_id),
    )
    tenant_db.commit()

    player = retrieve_player(tenant_db, player_id)
    if not player:
        abort(404, "player not found")

    tenant_db.close()

    return jsonify(
        SuccessResult(
            status=True,
            data={
                "player": PlayerDetail(
                    id=player.id, display_name=player.display_name, is_disqualified=player.is_disqualified
                )
            },
        )
    )


@dataclass
class CompetitionDetail:
    id: str
    title: str
    is_finished: bool


@app.route("/api/organizer/competitions/add", methods=["POST"])
def organizer_add_competitions():
    """
    テナント管理者向けAPI
    大会を追加する
    """
    viewer = parse_viewer()
    if viewer.role != ROLE_ORGANIZER:
        abort(403, "role organizer required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    title = request.values.get("title")

    now = int(datetime.now().timestamp())
    id = dispense_id()

    cur = tenant_db.cursor()
    cur.execute(
        "INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
        (id, viewer.tenant_id, title, None, now, now),
    )
    tenant_db.commit()

    tenant_db.close()

    return jsonify(
        SuccessResult(
            status=True,
            data={"competition": CompetitionDetail(id=id, title=title, is_finished=False)},
        )
    )


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
    viewer = parse_viewer()
    if viewer.role != ROLE_ORGANIZER:
        abort(403, "role organizer required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    competition = retrieve_competition(tenant_db, competition_id)
    if not competition:
        abort(404, "competition not found")

    if competition.finished_at:
        return jsonify(FailureResult(status=False, message="competition is finished")), 400

    form_file = request.files.get("scores")
    csv_reader = csv.reader(codecs.iterdecode(form_file, "utf-8"))
    header = next(csv_reader)

    if header != ["player_id", "score"]:
        abort(400, "invalid CSV headers")

    # DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
    lock_file_fd = flock_by_tenant_id(viewer.tenant_id)
    if not lock_file_fd:
        app.logger.warning("error flock_by_tenant_id")
        raise

    row_num = 0
    player_score_rows = []
    for row in csv_reader:
        row_num += 1
        if len(row) != 2:
            continue
        player_id = row[0]
        score_str = row[1]
        if retrieve_player(tenant_db, player_id) is None:
            # 存在しない参加者が含まれている
            continue

        score = int(score_str, 10)
        id = dispense_id()
        now = int(datetime.now().timestamp())
        player_score_rows.append(
            PlayerScoreRow(
                id=id,
                tenant_id=viewer.tenant_id,
                player_id=player_id,
                competition_id=competition_id,
                score=score,
                row_num=row_num,
                created_at=now,
                updated_at=now,
            )
        )

    cur = tenant_db.cursor()
    cur.execute(
        "DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?",
        (viewer.tenant_id, competition_id),
    )
    tenant_db.commit()

    for player_score_row in player_score_rows:
        cur.execute(
            "INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_num, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            (
                player_score_row.id,
                player_score_row.tenant_id,
                player_score_row.player_id,
                player_score_row.competition_id,
                player_score_row.score,
                player_score_row.row_num,
                player_score_row.created_at,
                player_score_row.updated_at,
            ),
        )
        tenant_db.commit()

    tenant_db.close()
    fcntl.flock(lock_file_fd, fcntl.LOCK_UN)

    return jsonify(SuccessResult(status=True, data={"rows": len(player_score_rows)}))


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


@dataclass
class PlayerScoreDetail:
    competition_title: str
    score: int


@app.route("/api/player/player/<player_id>", methods=["GET"])
def player_get_detail(player_id: str):
    """
    参加者向けAPI
    参加者の詳細情報を取得する
    """
    raise NotImplementedError()  # TODO


@dataclass
class CompetitionRank:
    rank: int
    score: int
    player_id: str
    player_display_name: str
    row_num: int


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
    tenant = retrieve_tenant_row_from_header()
    tenant_detail = TenantDetail(name=tenant.name, display_name=tenant.display_name)
    view = parse_viewer()

    return jsonify(tenant)


@dataclass
class InitializeHandlerResult:
    lang: str
    appeal: str


@app.route("/initialize", methods=["POST"])
def bench_initialize():
    """
    ベンチマーカー向けAPI
    ベンチマーカーが起動したときに最初に呼ぶ
    データベースの初期化などが実行されるため、スキーマを変更した場合などは適宜改変すること
    """
    try:
        subprocess.run([INITIALIZE_SCRIPT], shell=True)
    except subprocess.CalledProcessError as e:
        return f"error subprocess.run: {e.output} {e.stderr}"

    res = InitializeHandlerResult(
        lang="python",
        # 頑張ったポイントやこだわりポイントがあれば書いてください
        # 競技中の最後に計測したものを参照して、講評記事などで使わせていただきます
        appeal="",
    )
    return jsonify({"success": True, "data": res})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=3000, debug=True, threaded=True)
