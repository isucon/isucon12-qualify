import codecs
import csv
import fcntl
import os
import re
import subprocess
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Optional

import jwt
from flask import Flask, abort, jsonify, request
from sqlalchemy import create_engine
from sqlalchemy.exc import IntegrityError, OperationalError
from werkzeug.exceptions import HTTPException

from sqltrace import initialize_sql_logger

INITIALIZE_SCRIPT = "../sql/init.sh"
COOKIE_NAME = "isuports_session"
TENANT_DB_SCHEMA_FILE_PATH = "../sql/tenant/10_schema.sql"

ROLE_ADMIN = "admin"
ROLE_ORGANIZER = "organizer"
ROLE_PLAYER = "player"
ROLE_NONE = "none"

# 正しいテナント名の正規表現
TENANT_NAME_REGEXP = re.compile(r"^[a-z][a-z0-9-]{0,61}[a-z0-9]$")

admin_db = None

app = Flask(__name__)


def connect_admin_db():
    """管理用DBに接続する"""
    host = os.getenv("ISUCON_DB_HOST", "127.0.0.1")
    port = os.getenv("ISUCON_DB_PORT", 3306)
    user = os.getenv("ISUCON_DB_USER", "isucon")
    password = os.getenv("ISUCON_DB_PASSWORD", "isucon")
    database = os.getenv("ISUCON_DB_NAME", "isuports")

    return create_engine(f"mysql+mysqlconnector://{user}:{password}@{host}:{port}/{database}", pool_size=10)


def tenant_db_path(id: int) -> str:
    """テナントDBのパスを返す"""
    tenant_db_dir = os.getenv("ISUCON_TENANT_DB_DIR", "../tenant_db")
    return tenant_db_dir + f"/{id}.db"


def connect_to_tenant_db(id: int):
    """テナントDBに接続する"""
    path = tenant_db_path(id)
    engine = create_engine(f"sqlite:///{path}")
    return initialize_sql_logger(engine)


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
        try:
            res = admin_db.execute("REPLACE INTO id_generator (stub) VALUES (%s)", "a")
            id = res.lastrowid
            if id != 0:
                return hex(id)[2:]
        except OperationalError as e:  # deadlock
            last_err = e
            continue

    raise last_err


def run():
    global admin_db
    admin_db = connect_admin_db()

    app.run(host="0.0.0.0", port=3000, debug=True, threaded=True)


@app.errorhandler(Exception)
def error_handler(e):
    app.logger.error(f"error: {e}")
    if isinstance(e, HTTPException):
        return jsonify(FailureResult(status=False, message=e.description)), e.code
    else:
        return jsonify(FailureResult(status=False, message="Internal Server Error")), 500


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

    key_filename = os.getenv("ISUCON_JWT_KEY_FILE", "./public.pem")
    key = open(key_filename, "r").read()

    tenant = retrieve_tenant_row_from_header()
    try:
        token = jwt.decode(token_str, key, audience=tenant.name, algorithms=["RS256"])
    except jwt.ExpiredSignatureError:
        abort(401, "Signature has expire")

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
    row = admin_db.execute("SELECT * FROM tenant WHERE name = %s", tenant_name).fetchone()
    if not row:
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
    row = tenant_db.execute("SELECT * FROM player WHERE id = ?", id).fetchone()
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


def authorize_player(tenant_db, id: str):
    player = retrieve_player(tenant_db, id)
    if not player:
        abort(401, "player not found")

    if player.is_disqualified:
        abort(403, "player is disqualified")


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
    row = tenant_db.execute("SELECT * FROM competition WHERE id = ?", id).fetchone()
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

    yield lock_file_fd

    fcntl.flock(lock_file_fd, fcntl.LOCK_UN)


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

    if viewer.role != ROLE_ADMIN:
        abort(403, "admin role required")

    display_name = request.values.get("display_name")
    name = request.values.get("name")

    validate_tenant_name(name)

    now = int(datetime.now().timestamp())
    try:
        res = admin_db.execute(
            "INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (%s, %s, %s, %s)",
            name,
            display_name,
            now,
            now,
        )
        id = res.lastrowid
    except IntegrityError:  # duplicate entry
        abort(400, "duplicate tenant")

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


def billing_report_by_competition(tenant_db, tenant_id: int, competition_id: str):
    """大会ごとの課金レポートを計算する"""
    competition = retrieve_competition(tenant_db, competition_id)
    if not competition:
        raise

    visit_history_summary_rows = admin_db.execute(
        "SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = %s AND competition_id = %s GROUP BY player_id",
        tenant_id,
        competition.id,
    ).fetchall()

    billing_map = {}
    for vh in visit_history_summary_rows:
        # competition.finished_atよりもあとの場合は、終了後に訪問したとみなして大会開催内アクセス済みとみなさない
        if bool(competition.finished_at) and competition.finished_at < vh.min_created_at:
            continue
        billing_map[str(vh.player_id)] = "visitor"

    # player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    lock_file_fd = flock_by_tenant_id(tenant_id)
    if not lock_file_fd:
        app.logger.warning("error flock_by_tenant_id")
        raise

    # スコアを登録した参加者のIDを取得する
    scored_player_id_rows = tenant_db.execute(
        "SELECT DISTINCT(player_id) FROM player_score WHERE tenant_id = ? AND competition_id = ?",
        tenant_id,
        competition.id,
    ).fetchall()

    for pid in scored_player_id_rows:
        billing_map[str(pid.player_id)] = "player"

    player_count = 0
    visitor_count = 0
    if bool(competition.finished_at):
        for category in billing_map.values():
            if category == "player":
                player_count += 1
            if category == "visitor":
                visitor_count += 1

    return BillingReport(
        competition_id=competition.id,
        competition_title=competition.title,
        player_count=player_count,
        visitor_count=visitor_count,
        billing_player_yen=100 * player_count,
        billing_visitor_yen=10 * visitor_count,
        billing_yen=100 * player_count + 10 * visitor_count,
    )


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
    if request.host != os.getenv("ISUCON_ADMIN_HOSTNAME", "admin.t.isucon.dev"):
        abort(404, f"invalid hostname {request.host}")

    viewer = parse_viewer()
    if viewer.role != ROLE_ADMIN:
        abort(403, "admin role required")

    before = request.args.get("before")
    before_id = 0
    if before:
        before_id = int(before)

    # テナントごとに
    #   大会ごとに
    #     scoreに登録されているplayerでアクセスした人 * 100
    #     scoreに登録されているplayerでアクセスしていない人 * 50
    #     scoreに登録されていないplayerでアクセスした人 * 10
    #   を合計したものを
    # テナントの課金とする
    tenant_rows = admin_db.execute("SELECT * FROM tenant ORDER BY id DESC").fetchall()
    tenant_billings = []
    for tenant_row in tenant_rows:
        if before_id != 0 and before_id <= tenant_row.id:
            continue
        tenant_billing = TenantWithBilling(
            id=str(tenant_row.id), name=tenant_row.name, display_name=tenant_row.display_name, billing=0
        )
        tenant_db = connect_to_tenant_db(int(tenant_row.id))
        competition_rows = tenant_db.execute("SELECT * FROM competition WHERE tenant_id=?", tenant_row.id).fetchall()

        for competition_row in competition_rows:
            report = billing_report_by_competition(tenant_db, tenant_row.id, competition_row.id)
            tenant_billing.billing += report.billing_yen
        tenant_billings.append(tenant_billing)

        if len(tenant_billings) >= 10:
            break

    return jsonify(SuccessResult(status=True, data={"tenants": tenant_billings}))


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

    rows = tenant_db.execute(
        "SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC",
        viewer.tenant_id,
    ).fetchall()

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

        tenant_db.execute(
            "INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
            id,
            viewer.tenant_id,
            display_name,
            False,
            now,
            now,
        )

        player = retrieve_player(tenant_db, id)
        player_details.append(
            PlayerDetail(
                id=player.id,
                display_name=player.display_name,
                is_disqualified=player.is_disqualified,
            )
        )

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

    tenant_db.execute(
        "UPDATE player SET is_disqualified = ?, updated_at = ? WHERE id = ?",
        True,
        now,
        player_id,
    )

    player = retrieve_player(tenant_db, player_id)
    if not player:
        abort(404, "player not found")

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

    tenant_db.execute(
        "INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
        id,
        viewer.tenant_id,
        title,
        None,
        now,
        now,
    )

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
    viewer = parse_viewer()
    if viewer.role != ROLE_ORGANIZER:
        abort(403, "role organizer required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    competition = retrieve_competition(tenant_db, competition_id)
    if not competition:
        abort(404, "competition not found")

    now = int(datetime.now().timestamp())

    tenant_db.execute(
        "UPDATE competition SET finished_at = ?, updated_at = ? WHERE id = ?",
        now,
        now,
        competition_id,
    )

    return jsonify({"status": True})


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
            abort(400, f"player not found: {player_id}")

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

    tenant_db.execute(
        "DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?",
        viewer.tenant_id,
        competition_id,
    )

    for player_score_row in player_score_rows:
        tenant_db.execute(
            "INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_num, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            player_score_row.id,
            player_score_row.tenant_id,
            player_score_row.player_id,
            player_score_row.competition_id,
            player_score_row.score,
            player_score_row.row_num,
            player_score_row.created_at,
            player_score_row.updated_at,
        )

    return jsonify(SuccessResult(status=True, data={"rows": len(player_score_rows)}))


@app.route("/api/organizer/billing", methods=["GET"])
def organizer_get_billing():
    """
    テナント管理者向けAPI
    テナント内の課金レポートを取得する
    """
    viewer = parse_viewer()
    if viewer.role != ROLE_ORGANIZER:
        abort(403, "role organizer required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    rows = tenant_db.execute(
        "SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC",
        viewer.tenant_id,
    ).fetchall()
    if not rows:
        raise

    billing_reports = []
    for competition_row in rows:
        report = billing_report_by_competition(tenant_db, viewer.tenant_id, competition_row.id)
        billing_reports.append(report)

    return jsonify(SuccessResult(status=True, data={"reports": billing_reports}))


@app.route("/api/organizer/competitions", methods=["GET"])
def organizer_get_competitions():
    """
    主催者向けAPI
    大会の一覧を取得する
    """
    viewer = parse_viewer()
    if viewer.role != ROLE_ORGANIZER:
        abort(403, "role organizer required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    return competitions_handler(viewer, tenant_db)


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
    viewer = parse_viewer()
    if viewer.role != ROLE_PLAYER:
        abort(403, "role player required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    authorize_player(tenant_db, viewer.player_id)

    player = retrieve_player(tenant_db, player_id)
    if not player:
        abort(404, "player not found")

    competition_rows = []

    rows = tenant_db.execute("SELECT * FROM competition ORDER BY created_at ASC").fetchall()
    for row in rows:
        competition_rows.append(CompetitionRow(**row))

    # player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    lock_file_fd = flock_by_tenant_id(viewer.tenant_id)
    if not lock_file_fd:
        app.logger.warning("error flock_by_tenant_id")
        raise

    player_score_rows = []
    for competition_row in competition_rows:
        # 最後にCSVに登場したスコアを採用する = row_numが一番大きいもの
        row = tenant_db.execute(
            "SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? AND player_id = ? ORDER BY row_num DESC LIMIT 1",
            viewer.tenant_id,
            competition_row.id,
            player.id,
        ).fetchone()
        if not row:
            # 行がない = スコアが記録されてない
            continue
        player_score_rows.append(PlayerScoreRow(**row))

    player_score_details = []
    for player_score_row in player_score_rows:
        competition = retrieve_competition(tenant_db, player_score_row.competition_id)
        if not competition:
            continue
        player_score_details.append(
            PlayerScoreDetail(competition_title=competition.title, score=player_score_row.score)
        )

    return jsonify(
        SuccessResult(
            status=True,
            data={
                "player": PlayerDetail(
                    id=player.id, display_name=player.display_name, is_disqualified=player.is_disqualified
                ),
                "scores": player_score_details,
            },
        )
    )


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
    viewer = parse_viewer()
    if viewer.role != ROLE_PLAYER:
        abort(403, "role player required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    authorize_player(tenant_db, viewer.player_id)

    # 大会の存在確認
    competition = retrieve_competition(tenant_db, competition_id)
    if not competition:
        abort(404, "competition not found")

    now = int(datetime.now().timestamp())
    row = admin_db.execute("SELECT * FROM tenant WHERE id = %s", viewer.tenant_id).fetchone()
    if not row:
        return
    tenant_row = TenantRow(**row)

    admin_db.execute(
        "INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at) VALUES (%s, %s, %s, %s, %s)",
        viewer.player_id,
        tenant_row.id,
        competition_id,
        now,
        now,
    )

    rank_after = 0
    rank_after_str = request.args.get("rank_after")
    if rank_after_str:
        rank_after = int(rank_after_str)

    # player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    lock_file_fd = flock_by_tenant_id(viewer.tenant_id)
    if not lock_file_fd:
        app.logger.warning("error flock_by_tenant_id")
        raise

    player_score_rows = []
    rows = tenant_db.execute(
        "SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? ORDER BY row_num DESC",
        tenant_row.id,
        competition_id,
    ).fetchall()
    for row in rows:
        player_score_rows.append(PlayerScoreRow(**row))

    ranks = []
    scored_player_set = {}
    for player_score_row in player_score_rows:
        # player_scoreが同一player_id内ではrow_numの降順でソートされているので
        # 現れたのが2回目以降のplayer_idはより大きいrow_numでスコアが出ているとみなせる
        if scored_player_set.get(player_score_row.player_id) is not None:
            continue

        scored_player_set[player_score_row.player_id] = {}
        player = retrieve_player(tenant_db, player_score_row.player_id)
        if not player:
            raise
        ranks.append(
            CompetitionRank(
                rank=0,
                score=player_score_row.score,
                player_id=player.id,
                player_display_name=player.display_name,
                row_num=player_score_row.row_num,
            )
        )

    ranks.sort(key=lambda rank: (rank.score, rank.row_num), reverse=True)

    paged_ranks = []
    for i, rank in enumerate(ranks):
        if i < rank_after:
            continue
        paged_ranks.append(
            CompetitionRank(
                rank=i + 1,
                score=rank.score,
                player_id=rank.player_id,
                player_display_name=rank.player_display_name,
                row_num=0,
            )
        )
        if len(paged_ranks) >= 100:
            break

    return jsonify(
        SuccessResult(
            status=True,
            data={
                "competition": CompetitionDetail(
                    id=competition.id, title=competition.title, is_finished=bool(competition.finished_at)
                ),
                "ranks": paged_ranks,
            },
        )
    )


@app.route("/api/player/competitions", methods=["GET"])
def player_get_competitions():
    """
    参加者向けAPI
    大会の一覧を取得する
    """
    viewer = parse_viewer()
    if viewer.role != ROLE_PLAYER:
        abort(403, "role player required")

    tenant_db = connect_to_tenant_db(viewer.tenant_id)

    authorize_player(tenant_db, viewer.player_id)

    return competitions_handler(viewer, tenant_db)


def competitions_handler(viewer: Viewer, tenant_db):
    competition_rows = tenant_db.execute(
        "SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC", (viewer.tenant_id)
    ).fetchall()

    competition_details = []
    for competition_row in competition_rows:
        competition_details.append(
            CompetitionDetail(
                id=competition_row.id,
                title=competition_row.title,
                is_finished=bool(competition_row.finished_at),
            )
        )

    return jsonify(SuccessResult(status=True, data={"competitions": competition_details}))


@app.route("/api/me", methods=["GET"])
def get_me():
    """
    共通API
    JWTで認証した結果、テナントやユーザ情報を返す
    """
    tenant = retrieve_tenant_row_from_header()
    tenant_detail = TenantDetail(name=tenant.name, display_name=tenant.display_name)

    viewer = parse_viewer()
    if viewer.role == ROLE_ADMIN or viewer.role == ROLE_ORGANIZER:
        return jsonify(
            SuccessResult(
                status=True,
                data={
                    "tenant": tenant_detail,
                    "me": None,
                    "role": viewer.role,
                    "logged_in": True,
                },
            )
        )

    tenant_db = connect_to_tenant_db(viewer.tenant_id)
    player = retrieve_player(tenant_db, viewer.player_id)
    if not player:
        jsonify(
            SuccessResult(
                status=True,
                data={
                    "tenant": tenant_detail,
                    "me": None,
                    "role": ROLE_NONE,
                    "logged_in": False,
                },
            )
        )

    return jsonify(
        SuccessResult(
            status=True,
            data={
                "tenant": tenant_detail,
                "me": PlayerDetail(
                    id=player.id, display_name=player.display_name, is_disqualified=player.is_disqualified
                ),
                "role": viewer.role,
                "logged_in": True,
            },
        )
    )


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
    run()
