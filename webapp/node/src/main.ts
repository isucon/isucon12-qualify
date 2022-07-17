import express, { Request, Response, NextFunction } from 'express'
import cookieParser from 'cookie-parser'
import bodyParser from 'body-parser'
import mysql, { RowDataPacket, QueryError, OkPacket } from 'mysql2/promise'
import childProcess from 'child_process'
import { readFile } from 'fs/promises'
import jwt from 'jsonwebtoken'
import util from 'util'
import path from 'path'
import sqlite3 from 'sqlite3'
import { open, Database } from 'sqlite'
import { openSync, closeSync } from 'fs'
import fsExt from 'fs-ext'

const exec = util.promisify(childProcess.exec)
const flock = util.promisify(fsExt.flock)

// constants
const tenantDBSchemaFilePath = "../sql/tenant/10_schema.sql"
const initializeScript       = "../sql/init.sh"
const cookieName             = "isuports_session"

const RoleAdmin     = "admin"
const RoleOrganizer = "organizer"
const RolePlayer    = "player"
const RoleNone      = "none"

const tenantNameRegexp = /^[a-z][a-z0-9-]{0,61}[a-z0-9]$/

// 環境変数を取得する、なければデフォルト値を返す
function getEnv(key: string, defaultValue: string): string {
  const val = process.env[key]
  if (val !== undefined) {
    return val
  }

  return defaultValue
}

// 管理用DBに接続する
const dbConfig = {
  host: process.env['ISUCON_DB_HOST'] ?? '127.0.0.1',
  port: Number(process.env['ISUCON_DB_PORT'] ?? 3306),
  user: process.env['ISUCON_DB_USER'] ?? 'isucon',
  password: process.env['ISUCON_DB_PASSWORD'] ?? 'isucon',
  database: process.env['ISUCON_DB_NAME'] ?? 'isucon_listen80',
}
const adminDB = mysql.createPool(dbConfig)

// テナントDBのパスを返す
function tenantDBPath(id: number): string {
  const tenantDBDir = getEnv("ISUCON_TENANT_DB_DIR", "../tenant_db")
	return path.join(tenantDBDir, `${id.toString()}.db`)
}

// テナントDBに接続する
async function connectToTenantDB(id: number): Promise<Database> {
  const p = tenantDBPath(id)
  let db: Database
  try {
    db = await open({
      filename: p,
      driver: sqlite3.Database,
    })
  } catch (error: any) {
    throw new Error(`failed to open tenant DB: ${error}`)
  }

  return db
}

// テナントDBを新規に作成する
async function createTenantDB(id: number): Promise<Error | undefined> {
  const p = tenantDBPath(id)

  try {
    await exec(`sh -c "sqlite3 ${p} < ${tenantDBSchemaFilePath}"`)
  } catch (error: any) {
    console.log
    return new Error(`failed to exec "sqlite3 ${p} < ${tenantDBSchemaFilePath}", out=${error.stderr}`)
  }
}

// システム全体で一意なIDを生成する
async function dispenseID(): Promise<string> {
  let id = 0
  let lastErr: any
  for (const _ of Array(100)) {
    try {
      const [result] = await adminDB.execute<OkPacket>(
        'REPLACE INTO id_generator (stub) VALUES (?)',
        ['a'],
      )

      id = result.insertId
      break
    } catch (error: any) {
      if (error.errno && error.errno === 1213) { // deadlock
        lastErr = error
      }
    }
  }
  if (id != 0) {
    return id.toString(16)
  }

  throw new Error('error REPLACE INTO id_generator: ${lastErr.toString()}')
}

// カスタムエラーハンドラにステータスコード拾ってもらうエラー型
class ErrorWithStatus extends Error {
  public status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = new.target.name;
    this.status = status;
  }
}

// 内部型定義
type TenantWithBilling = {
  id: string
  name: string
  display_name: string
  billing: number
}

type BillingReport = {
  competition_id: string
  competition_title: string
  player_count: number        // スコアを登録した参加者数
  visitor_count: number       // ランキングを閲覧だけした(スコアを登録していない)参加者数
  billing_player_yen: number  // 請求金額 スコアを登録した参加者分
  billing_visitor_yen: number // 請求金額 ランキングを閲覧だけした(スコアを登録していない)参加者分
  billing_yen: number         // 合計請求金額
}

type PlayerDetail = {
  id: string
  display_name: string
  is_disqualified: boolean
}

type CompetitionDetail = {
  id: string
  title: string
  is_finished: boolean
}

// レスポンス型定義
type TenantsAddResult = {
  tenant: TenantWithBilling
}
type InitializeResult = {
  lang: string
}

type PlayersListResult = {
  players: PlayerDetail[]
}
type PlayersAddResult = {
  players: PlayerDetail[]
}
type PlayerDisqualifiedResult = {
  player: PlayerDetail
}

type CompetitionsAddResult = {
  competition: CompetitionDetail
}

// DB型定義
interface TenantRow {
  id: number
  name: string
  display_name: string
  created_at?: number
  updated_at?: number
}

interface CompetitionRow {
	tenant_id: number
  id: string
  title: string
  finished_at: number | null
  created_at: number
  updated_at: number
}

interface VisitHistorySummaryRow {
  player_id: string
  min_created_at: number
}

interface PlayerRow {
  tenant_id: number
  id: string
  display_name: string
  is_disqualified: number
  created_at: number
  updated_at: number
}


const app = express()
app.use(express.json())
app.use(cookieParser())
app.use(bodyParser.urlencoded({ extended: false }));

//@ts-ignore see: https://expressjs.com/en/advanced/best-practice-performance.html#handle-exceptions-properly
const wrap = fn => (...args) => fn(...args).catch(args[2])

// アクセスしてきた人の情報
type Viewer = {
  role: string
  playerId: string
  tenantName: string
  tenantId: number
}

// リクエストヘッダをパースしてViewerを返す
async function parseViewer(req: Request): Promise<Viewer> {
  const tokenStr = req.cookies[cookieName]
  if (!tokenStr) {
    throw new ErrorWithStatus(401,
      `cookie ${cookieName} is not found`,
    )
  }

  const keyFilename = getEnv('ISUCON_JWT_KEY_FILE', '../public.pem')
  const cert = await readFile(keyFilename)

  let token: jwt.JwtPayload
  try {
    token = jwt.verify(tokenStr, cert, {
      algorithms: ['RS256'],
    }) as jwt.JwtPayload
  } catch (error: any) {
    throw new ErrorWithStatus(401, error.toString())
  }

  if (!token['sub']) {
    throw new ErrorWithStatus(401,
      `invalid token: subject is not found in token: ${tokenStr}`,
    )
  }
  const subject = token.sub

  const tr: string | undefined = token['role']
  if (!tr) {
    throw new ErrorWithStatus(401,
      `invalid token: role is not found: ${tokenStr}`
    )
  }

  let role = ''
  switch (tr) {
  case RoleAdmin:
  case RoleOrganizer:
  case RolePlayer:
    role = tr
    break

  default:
    throw new ErrorWithStatus(401,
      `invalid token: ${tr} is invalid role: ${tokenStr}"`
    )
  }

  // aud は1要素で、テナント名が入っている
  const aud: string[] | undefined = token.aud as string[]
  if (!aud || aud.length != 1) {
    throw new ErrorWithStatus(401,
      `invalid token: aud field is few or too much: ${tokenStr}`
    )
  }

  const tenant = await retrieveTenantRowFromHeader(req)
  if (!tenant) {
    throw new ErrorWithStatus(401, 'tenant not found')
  }
  if (tenant.name === 'admin' && role != RoleAdmin) {
    throw new ErrorWithStatus(401, 'tenant not found')
  }
  if (tenant.name !== aud[0]) {
    throw new ErrorWithStatus(401,
      `invalid token: tenant name is not match with ${req.hostname}: ${tokenStr}`,
    )
  }

  return {
    role: role,
    playerId: subject,
    tenantName: tenant.name,
    tenantId: tenant.id ?? 0,
  } 
}

async function retrieveTenantRowFromHeader(req: Request): Promise<TenantRow | undefined> {
  // JWTに入っているテナント名とHostヘッダのテナント名が一致しているか確認
  const baseHost = getEnv('ISUCON_BASE_HOSTNAME', '.t.isucon.dev')
  const tenantName = req.hostname.replace(baseHost, '')

  // SaaS管理者用ドメイン
  if (tenantName === 'admin') {
    return {
      id: 0,
      name: 'admin',
      display_name: 'admin',
    }
  }

  // テナントの存在確認
  const [[tenant]] = await adminDB.query<(TenantRow & RowDataPacket)[]>(
    'SELECT * FROM tenant WHERE name = ?',
    [tenantName]
  )
  if (!tenant) {
    return // no row
  }

  return tenant
}

// 参加者を取得する
async function retrievePlayer(tenantDB: Database, id: string): Promise<PlayerRow | undefined> {
  try {
    const playerRow = await tenantDB.get<PlayerRow>(
      'SELECT * FROM player WHERE id = ?',
      id,
    )
    if (!playerRow) {
      return
    }
    return playerRow
  } catch(error) {
    throw new Error('error Select player: id=${id} ${error.toString()}')
  }
}

// 参加者を認可する
// 参加者向けAPIで呼ばれる
async function authorizePlayer(tenantDB: Database, id: string): Promise<boolean> {
  try {
    const player = await retrievePlayer(tenantDB, id)
    if (!player) {
      throw new ErrorWithStatus(401, 'player not found')
    }
  } catch (error: any) {
    return false
  }
  return true
}

// 大会を取得する
async function retrieveCompetition(tenantDB: Database, id: string): Promise<CompetitionRow> {
  const competitionRow = await tenantDB.get<CompetitionRow>(
    'SELECT * FROM competition WHERE id = ?',
    id,
  )
  if (!competitionRow) {
    throw new Error('error Select competition: id=${id}')
  }
  return competitionRow
}

// 排他ロックのためのファイル名を生成する
function lockFilePath(tenantId: number): string {
  const tenantDBDir = getEnv('ISUCON_TENANT_DB_DIR', '../tenant_db')
  return path.join(tenantDBDir, `${tenantId}.lock`)
}

// 排他ロックする
async function flockByTenantID(tenantId: number): Promise<() => Promise<void>> {
  const p = lockFilePath(tenantId)

  const fd = openSync(p, 'w+')
  try {
    await flock(fd, fsExt.constants.LOCK_EX)
  } catch (error: any) {
    throw new Error(`error flock: path=${p}, ${error.toString()}`)
  }

  const close = async () => {
    await flock(fd, fsExt.constants.LOCK_UN)
    closeSync(fd)
  }
  return close
}


// SaaS管理者向けAPI
// テナントを追加する
// POST /api/admin/tenants/add
app.post('/api/admin/tenants/add', wrap(async (req: Request, res: Response) => {
  try {
    const viewer = await parseViewer(req)
    if (viewer.tenantName !== 'admin') {
      // admin: SaaS管理者用の特別なテナント名
      throw new ErrorWithStatus(404, `${viewer.tenantName} has not this API`)
    }
    if (viewer.role !== RoleAdmin) {
      throw new ErrorWithStatus(403, 'admin role required')
    }

    const { name, display_name } = req.body
    if (!validateTenantName(name)) {
      throw new ErrorWithStatus(400, `invalid tenant name: ${name}` )
    }

    const now = Math.floor(new Date().getTime() / 1000)
    let id: number
    try {
      const [result] = await adminDB.execute<OkPacket>(
        'INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)',
        [name, display_name, now, now],
      )
      id = result.insertId
    } catch (error: any) {
      if (error.errno && error.errno === 1062) {
        throw new ErrorWithStatus(400, 'duplicate tenant')
      }
      throw new Error(`error Insert tenant: name=${name}, displayName=${display_name}, createdAt=${now}, updatedAt=${now}, ${error.toString()}`)
    }

    const error = await createTenantDB(id)
    if (error) {
      throw new Error(`error createTenantDB: id=${id} name=${name} ${error.toString()}`)
    }

    const data: TenantsAddResult = {
      tenant: {
        id: id.toString(),
        name,
        display_name,
        billing: 0,
      }
    }

    res.status(200).json({
      status: true,
      data,
    })
  } catch (error: any) {
    if (error.status) {
      throw error // rethrow
    }
    throw new ErrorWithStatus(500, error.toString())
  }
}))

// テナント名が規則に沿っているかチェックする
function validateTenantName(name: string): boolean {
  if (name.match(tenantNameRegexp)) {
    return true
  }
  return false
}

// 大会ごとの課金レポートを計算する
async function billingReportByCompetition(tenantDB: Database, tenantId: number, competitionId: string): Promise<BillingReport> {
  const comp = await retrieveCompetition(tenantDB, competitionId)

  // ランキングにアクセスした参加者のIDを取得する
  const [vhs] = await adminDB.query<(VisitHistorySummaryRow & RowDataPacket)[]>(
    'SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = ? AND competition_id = ? GROUP BY player_id',
    [tenantId, comp.id],
  )

  const billingMap: {[playerId: string]: string} = {}
  for (const vh of vhs) {
    // competition.finished_atよりもあとの場合は、終了後に訪問したとみなして大会開催内アクセス済みとみなさない
    if (comp.finished_at !== null && comp.finished_at < vh.min_created_at) {
      continue
    }
    billingMap[vh.player_id] = 'visitor'
  }

  // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
  const unlock = await flockByTenantID(tenantId)
  try {
    // スコアを登録した参加者のIDを取得する
    const scoredPlayerIds = await tenantDB.all<{player_id: string}[]>(
      'SELECT DISTINCT(player_id) FROM player_score WHERE tenant_id = ? AND competition_id = ?',
      tenantId, comp.id,
    )
    for (const pid of scoredPlayerIds) {
      // スコアが登録されている参加者
      billingMap[pid.player_id] = 'player'
    }

    // 大会が終了している場合のみ請求金額が確定するので計算する
    const counts = {
      player: 0,
      visitor: 0,
    }
    if (comp.finished_at) {
      for (const category of Object.values(billingMap)) {
        switch (category) {
          case 'player':
            counts.player++
            break
          case 'visitor':
            counts.visitor++
            break
        }
      }
    }

    return {
      competition_id: comp.id,
      competition_title: comp.title,
      player_count: counts.player,
      visitor_count: counts.visitor,
      billing_player_yen: 100 * counts.player,
      billing_visitor_yen: 10 * counts.visitor, 
      billing_yen: 100 * counts.player + 10 * counts.visitor,
    }
  
  } catch (error: any) {
    throw new Error(`error Select count player_score: tenantId=${tenantId}, competitionId=${comp.id}, ${error.toString()}`)
  } finally {
    await unlock()
  }
}

// SaaS管理者用API
// テナントごとの課金レポートを最大10件、テナントのid降順で取得する
// GET /api/admin/tenants/billing
// URL引数beforeを指定した場合、指定した値よりもidが小さいテナントの課金レポートを取得する
app.get('/api/admin/tenants/billing', wrap(async (req: Request, res: Response) => {
  try {
    if (req.hostname != getEnv('ISUCON_ADMIN_HOSTNAME', 'admin.t.isucon.dev')) {
      throw new ErrorWithStatus(404, `invalid hostname ${req.hostname}`)
    }
  
    const viewer = await parseViewer(req)
    if (viewer.role !== RoleAdmin) {
      throw new ErrorWithStatus(403, 'admin role required')
    }
  
    const before = req.query.before as string
    const beforeId = before ? parseInt(before, 10) : 0
    if (isNaN(beforeId)) {
      throw new ErrorWithStatus(400, `failed to parse query parameter 'before'=${before}`)
    }
  
    // テナントごとに
    //   大会ごとに
    //     scoreに登録されているplayerでアクセスした人 * 100
    //     scoreに登録されていないplayerでアクセスした人 * 10
    //   を合計したものを
    // テナントの課金とする
    const ts: TenantRow[] = []
    const tenantBillings: TenantWithBilling[] = []
    try {
      const [tenants] = await adminDB.query<(TenantRow & RowDataPacket)[]>(
        'SELECT * FROM tenant ORDER BY id DESC',
      )
      ts.push(...tenants)
    } catch (error: any) {
      throw new Error(`error Select tenant: ${error.toString()}`)
    }
  
    for (const tenant of ts) {
      if (beforeId != 0 && beforeId <= tenant.id) {
        continue
      }
  
      const tb: TenantWithBilling = {
        id: tenant.id.toString(),
        name: tenant.name,
        display_name: tenant.display_name,
        billing: 0,
      }
  
      const tenantDB = await connectToTenantDB(tenant.id)
      try {
        const competitions = await tenantDB.all<CompetitionRow[]>(
          'SELECT * FROM competition WHERE tenant_id = ?',
          tenant.id,
        )

        for (const comp of competitions) {
          const report = await billingReportByCompetition(tenantDB, tenant.id, comp.id)
          tb.billing += report.billing_yen
        }
      } catch (error) {
        // TODO
        throw error
      } finally {
        tenantDB.close()
      }

      tenantBillings.push(tb)
      if (tenantBillings.length >= 10) {
        break
      }
    }
  
    res.status(200).json({
      status: true,
      data: {
        tenants: tenantBillings,
      }
    })  
  } catch (error: any) {
    if (error.status) {
      throw error // rethrow
    }
    throw new ErrorWithStatus(500, error.toString())
  }
}))


// テナント管理者向けAPI - 参加者追加、一覧、失格

// テナント管理者向けAPI
// GET /api/organizer/players
// 参加者一覧を返す
app.get('/api/organizer/players', wrap(async (req: Request, res: Response) => {
  try {
    const viewer = await parseViewer(req)
    if (viewer.role !== RoleOrganizer) {
      throw new ErrorWithStatus(403, 'role organizer required')
    }

    const pds: PlayerDetail[] = []

    const tenantDB = await connectToTenantDB(viewer.tenantId)
    try {
      const pls = await tenantDB.all<PlayerRow[]>(
        'SELECT * FROM player WHERE tenant_id = ? ORDER BY created_at DESC',
        viewer.tenantId,
      )

      pls.forEach(player => {
        pds.push({
          id: player.id,
          display_name: player.display_name,
          is_disqualified: !!player.is_disqualified,
        })
      })

    } catch (error) {
      throw new Error(`error Select player: ${error}`)
    } finally {
      tenantDB.close()
    }

    const data: PlayersListResult = {
      players: pds,
    }

    res.status(200).json({
      status: true,
      data,
    })
  } catch (error: any)  {
    if (error.status) {
      throw error // rethrow
    }
    throw new ErrorWithStatus(500, error.toString())
  }
}))

app.post('/api/organizer/players/add', wrap(async (req: Request, res: Response) => {
  try {
    const viewer = await parseViewer(req)
    if (viewer.role !== RoleOrganizer) {
      throw new ErrorWithStatus(403, 'role organizer required')
    }

    const pds: PlayerDetail[] = []
    const tenantDB = await connectToTenantDB(viewer.tenantId)
    try {
      const displayNames: string[] = req.body['display_name[]']

      for (const displayName of displayNames) {
        const id = await dispenseID()
        const now = Math.floor(new Date().getTime() / 1000)

        try {
          await tenantDB.run(
            'INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)',
            id, viewer.tenantId, displayName, false, now, now,
          )
        } catch (error) {
          throw new Error(`error Insert player at tenantDB: tenantId=${viewer.tenantId} id=${id}, displayName=${displayName}, isDisqualified=false, createdAt=${now}, updatedAt=${now}, ${error}`)
        }

        const player = await retrievePlayer(tenantDB, id)
        if (!player) {
          throw new Error('error retrievePlayer id=${id}')
        }
        pds.push({
          id: player.id,
          display_name: player.display_name,
          is_disqualified: !!player.is_disqualified,
        })
      }

    } catch (error) {
      throw error
    } finally {
      tenantDB.close()
    }

    const data: PlayersAddResult = {
      players: pds,
    }

    res.status(200).json({
      status: true,
      data,
    })
  } catch (error: any)  {
    if (error.status) {
      throw error // rethrow
    }
    throw new ErrorWithStatus(500, error.toString())
  }
}))

// テナント管理者向けAPI
// POST /api/organizer/player/:player_id/disqualified
// 参加者を失格にする
app.post('/api/organizer/player/:playerId/disqualified', wrap(async (req: Request, res: Response) => {
  try {
    const viewer = await parseViewer(req)
    if (viewer.role !== RoleOrganizer) {
      throw new ErrorWithStatus(403, 'role organizer required')
    }

    const tenantDB = await connectToTenantDB(viewer.tenantId)
    const { playerId } = req.params
    const now = Math.floor(new Date().getTime() / 1000)
    let pd: PlayerDetail
    try {
      try {
        await tenantDB.run(
          'UPDATE player SET is_disqualified = ?, updated_at = ? WHERE id = ?',
          true, now, playerId,
        )  
      } catch (error) {
        throw new Error(`error Update player: isDisqualified=true, updatedAt=${now}, id=${playerId}, ${error}`)
      }

      const player = await retrievePlayer(tenantDB, playerId)
      if (!player) {
        // 存在しないプレイヤー
        throw new ErrorWithStatus(404, 'player not found')
      }
      pd = {
        id: player.id,
        display_name: player.display_name,
        is_disqualified: !!player.is_disqualified,
      }

    } catch (error: any) {
      if (error.status) {
        throw error // rethrow
      }
      throw new Error(`error Select player: ${error}`)
    } finally {
      tenantDB.close()
    }

    const data: PlayerDisqualifiedResult = {
      player: pd,
    }

    res.status(200).json({
      status: true,
      data,
    })
  } catch (error: any)  {
    if (error.status) {
      throw error // rethrow
    }
    throw new ErrorWithStatus(500, error.toString())
  }
}))

// テナント管理者向けAPI - 大会管理

// テナント管理者向けAPI
// POST /api/organizer/competitions/add
// 大会を追加する
app.post('/api/organizer/competitions/add', wrap(async (req: Request, res: Response) => {
  try {
    const viewer = await parseViewer(req)
    if (viewer.role !== RoleOrganizer) {
      throw new ErrorWithStatus(403, 'role organizer required')
    }

    const tenantDB = await connectToTenantDB(viewer.tenantId)
    const { title } = req.body
    const now = Math.floor(new Date().getTime() / 1000) 
    const id = await dispenseID()
    try {
      try {
        await tenantDB.run(
          'INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)',
          id, viewer.tenantId, title, null, now, now,
        )
      } catch (error) {
        throw new Error(`error Insert competition: id=${id}, tenant_id=${viewer.tenantId}, title=${title}, finishedAt=null, createdAt=${now}, updatedAt=${now}, ${error}`)
      }


    } catch (error) {
      throw new Error(`error Select player: ${error}`)
    } finally {
      tenantDB.close()
    }

    const data: CompetitionsAddResult = {
      competition: {
        id,
        title,
        is_finished: false,
      },
    }

    res.status(200).json({
      status: true,
      data,
    })
  } catch (error: any)  {
    if (error.status) {
      throw error // rethrow
    }
    throw new ErrorWithStatus(500, error.toString())
  }
}))

// テナント管理者向けAPI
// POST /api/organizer/competition/:competition_id/finish
// 大会を終了する
app.post('/api/organizer/competition/:competitionId/finish', wrap(async (req: Request, res: Response) => {
  try {
    const viewer = await parseViewer(req)
    if (viewer.role !== RoleOrganizer) {
      throw new ErrorWithStatus(403, 'role organizer required')
    }

    const tenantDB = await connectToTenantDB(viewer.tenantId)
    const { competitionId } = req.params
    if (!competitionId) {
      throw new ErrorWithStatus(400, 'competition_id required')
    }
    const now = Math.floor(new Date().getTime() / 1000) 

    try {
      const competition = await retrieveCompetition(tenantDB, competitionId)
      if (!competition) {
        // TODO
        throw new ErrorWithStatus(404, 'competition not found')
      }

      await tenantDB.run(
        'UPDATE competition SET finished_at = ?, updated_at = ? WHERE id = ?',
        now, now, competitionId,
      )

    } catch (error) {
      // TODO
      throw error
    } finally {
      tenantDB.close()
    }

    res.status(200).json({
      status: true,
    })
  } catch (error: any)  {
    if (error.status) {
      throw error // rethrow
    }
    throw new ErrorWithStatus(500, error.toString())
  }
}))

// テナント管理者向けAPI
// POST /api/organizer/competition/:competition_id/score
// 大会のスコアをCSVでアップロードする
app.post('/api/organizer/competition/:competitionId/score', wrap(async (req: Request, res: Response) => {
/*
  ctx := context.Background()
	v, err := parseViewer(c)
	if err != nil {
		return fmt.Errorf("error parseViewer: %w", err)
	}
	if v.role != RoleOrganizer {
		return echo.NewHTTPError(http.StatusForbidden, "role organizer required")
	}

	tenantDB, err := connectToTenantDB(v.tenantID)
	if err != nil {
		return err
	}
	defer tenantDB.Close()

	competitionID := c.Param("competition_id")
	if competitionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "competition_id required")
	}
	comp, err := retrieveCompetition(ctx, tenantDB, competitionID)
	if err != nil {
		// 存在しない大会
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "competition not found")
		}
		return fmt.Errorf("error retrieveCompetition: %w", err)
	}
	if comp.FinishedAt.Valid {
		res := FailureResult{
			Success: false,
			Message: "competition is finished",
		}
		return c.JSON(http.StatusBadRequest, res)
	}

	fh, err := c.FormFile("scores")
	if err != nil {
		return fmt.Errorf("error c.FormFile(scores): %w", err)
	}
	f, err := fh.Open()
	if err != nil {
		return fmt.Errorf("error fh.Open FormFile(scores): %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	headers, err := r.Read()
	if err != nil {
		return fmt.Errorf("error r.Read at header: %w", err)
	}
	if !reflect.DeepEqual(headers, []string{"player_id", "score"}) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid CSV headers")
	}

	// / DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
	fl, err := flockByTenantID(v.tenantID)
	if err != nil {
		return fmt.Errorf("error flockByTenantID: %w", err)
	}
	defer fl.Close()
	var rowNum int64
	playerScoreRows := []PlayerScoreRow{}
	for {
		rowNum++
		row, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error r.Read at rows: %w", err)
		}
		if len(row) != 2 {
			return fmt.Errorf("row must have two columns: %#v", row)
		}
		playerID, scoreStr := row[0], row[1]
		if _, err := retrievePlayer(ctx, tenantDB, playerID); err != nil {
			// 存在しない参加者が含まれている
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(
					http.StatusBadRequest,
					fmt.Sprintf("player not found: %s", playerID),
				)
			}
			return fmt.Errorf("error retrievePlayer: %w", err)
		}
		var score int64
		if score, err = strconv.ParseInt(scoreStr, 10, 64); err != nil {
			return echo.NewHTTPError(
				http.StatusBadRequest,
				fmt.Sprintf("error strconv.ParseUint: scoreStr=%s, %s", scoreStr, err),
			)
		}
		id, err := dispenseID(ctx)
		if err != nil {
			return fmt.Errorf("error dispenseID: %w", err)
		}
		now := time.Now().Unix()
		playerScoreRows = append(playerScoreRows, PlayerScoreRow{
			ID:            id,
			TenantID:      v.tenantID,
			PlayerID:      playerID,
			CompetitionID: competitionID,
			Score:         score,
			RowNum:        rowNum,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}

	if _, err := tenantDB.ExecContext(
		ctx,
		"DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?",
		v.tenantID,
		competitionID,
	); err != nil {
		return fmt.Errorf("error Delete player_score: tenantID=%d, competitionID=%s, %w", v.tenantID, competitionID, err)
	}
	for _, ps := range playerScoreRows {
		if _, err := tenantDB.NamedExecContext(
			ctx,
			"INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_num, created_at, updated_at) VALUES (:id, :tenant_id, :player_id, :competition_id, :score, :row_num, :created_at, :updated_at)",
			ps,
		); err != nil {
			return fmt.Errorf(
				"error Insert player_score: id=%s, tenant_id=%d, playerID=%s, competitionID=%s, score=%d, rowNum=%d, createdAt=%d, updatedAt=%d, %w",
				ps.ID, ps.TenantID, ps.PlayerID, ps.CompetitionID, ps.Score, ps.RowNum, ps.CreatedAt, ps.UpdatedAt, err,
			)

		}
	}

	return c.JSON(http.StatusOK, SuccessResult{
		Success: true,
		Data:    ScoreHandlerResult{Rows: int64(len(playerScoreRows))},
	})
*/
}))

// テナント管理者向けAPI
// GET /api/organizer/billing
// テナント内の課金レポートを取得する
app.get('/api/organizer/billing', wrap(async (req: Request, res: Response) => {
}))

// 主催者向けAPI
// GET /api/organizer/competitions
// 大会の一覧を取得する
app.get('/api/organizer/competitions', wrap(async (req: Request, res: Response) => {
}))

// 参加者向けAPI
// GET /api/player/player/:player_id
// 参加者の詳細情報を取得する
app.get('/api/player/player/:playerId', wrap(async (req: Request, res: Response) => {
}))

// 参加者向けAPI
// GET /api/player/competition/:competition_id/ranking
// 大会ごとのランキングを取得する
app.get('/api/player/competition/:competitionId/ranking', wrap(async (req: Request, res: Response) => {
}))

// 参加者向けAPI
// GET /api/player/competitions
// 大会の一覧を取得する
app.get('/api/player/competitions', wrap(async (req: Request, res: Response) => {
}))

// 共通API
// GET /api/me
// JWTで認証した結果、テナントやユーザ情報を返す
app.get('/api/me', wrap(async (req: Request, res: Response) => {
}))

// ベンチマーカー向けAPI
// POST /initialize
// ベンチマーカーが起動したときに最初に呼ぶ
// データベースの初期化などが実行されるため、スキーマを変更した場合などは適宜改変すること
app.post('/initialize', wrap(async (req: Request, res: Response, next: NextFunction) => {
  try {
    const result = await exec(initializeScript)
  
    const data: InitializeResult = {
      lang: 'node',
    }
    res.status(200).json({
      status: true,
      data,
    })
  } catch (error: any) {
    throw new ErrorWithStatus(500, `initialize failed: ${error.toString()}`)
  }
}))

// エラー処理関数
app.use((err: ErrorWithStatus, req: Request, res: Response, next: NextFunction) => {
  console.error('error occurred: status=' + err.status + ' reason=' + err.message)
  res.status(err.status).json({
    status: false,
  })
})

const port = getEnv('SERVER_APP_PORT', '3000')
console.log('starting isuports server on :' + port + ' ...')
app.listen(port)

