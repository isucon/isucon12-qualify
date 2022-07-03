use sqlx::ConnectOptions;
use std::path::{Path, PathBuf};
use std::str::FromStr;

use actix_web::web;
use lazy_static::lazy_static;
use regex::Regex;
use sqlx::mysql::MySqlConnectOptions;
use sqlx::sqlite::SqliteConnectOptions;

const TENANT_DB_SCHEMA_FILE_PATH: &str = "../sql/tenant/10_schema.sql";
const INITIALIZE_SCRIPT: &str = "..sql/init.sh";
const COOKIE_NAME: &str = "isuports_session";

const ROLE_ADMIN: &str = "admin";
const ROLE_ORGANIZER: &str = "organizer";
const ROLE_PLAYER: &str = "player";
const ROLE_NONE: &str = "none";

lazy_static! {
    // 正しいテナント名の正規表現
    static ref TENANT_NAME_REGEXP: Regex = Regex::new("^[a-z][a-z0-9-]{0,61}[a-z0-9]$").unwrap();
    // TODO: static ref ADMIN_DB:

    static ref SQLITE_DRIBER_NAME: String = "sqlite3".to_string();
}

// 環境変数を取得する, なければデフォルト値を返す
fn get_env(key: &str, default: &str) -> String {
    match std::env::var(key) {
        Ok(val) => val,
        Err(_) => default.to_string(),
    }
}

// 管理用DBに接続する
async fn connect_admin_db() -> Result<sqlx::MySqlConnection, sqlx::Error> {
    let host: String =
        get_env("ISUCON_DB_HOST", "127.0.0.1") + ":" + &get_env("ISUCON_DB_PORT", "3306");

    MySqlConnectOptions::new()
        .host(&host)
        .username(&get_env("ISUCON_DB_USER", "isucon"))
        .password(&get_env("ISUCON_DB_PASSWORD", "isucon"))
        .database(&get_env("ISUCON_DB_NAME", "isuports"))
        .connect()
        .await
}

// テナントDBのパスを返す
fn tenant_db_path(id: i64) -> PathBuf {
    let tenant_db_dir = get_env("ISUCON_TENANT_DB_DIR", "../tenant_db");
    return Path::new(&tenant_db_dir).join(format!("{}.db", id));
}

// テナントDBに接続する
async fn connect_to_tenant_db(id: i64) -> Result<sqlx::SqliteConnection, sqlx::Error> {
    let p = tenant_db_path(id);
    let uri = format!("sqlite:{}?mode=rw", p.to_str().unwrap());
    SqliteConnectOptions::from_str(&uri)?.connect().await
    // TODO: sqliteDriverNameを使ってないのをなおす
}

// テナントDBを新規に作成する
async fn create_tenant_db(id: i64) {
    let p = tenant_db_path(id);
    let cmd = tokio::process::Command::new("sh").arg("-c").arg(format!(
        "sqlite3 {} < {}",
        p.to_str().unwrap(),
        TENANT_DB_SCHEMA_FILE_PATH
    ));
    let out = cmd.output().await.expect(
        format!(
            "failed to exec sqlite3 {} < {}",
            p.to_str().unwrap(),
            TENANT_DB_SCHEMA_FILE_PATH
        )
        .as_str(),
    );
}

// システム全体で一意なIDを生成する
fn dispense_id() {}

#[actix_web::main]
async fn run() -> std::io::Result<()> {
    // sqliteのクエリログを出力する設定
    // 環境変数 ISUCON_SQLITE_TRACE_FILEを設定すると, そのファイルにクエリログをJSON形式で出力する
    // 未設定なら出力しない
    // sqltrace.rsを参照

    let pool = sqlx::mysql::MySqlPoolOptions::new()
        .max_connections(10)
        .connect_with()
        .await?
        .expect("failed to connect db");

    let server = actix_web::HttpServer::new(move || {
        actix_web::App::new()
            .service(player_handler)
            .service(competition_ranking_handler)
            .service(player_competition_handler)
            .service(me_handler)
            .service(initialize_handler)
    });

    server.run().await
}
