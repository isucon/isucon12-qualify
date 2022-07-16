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

const dbConfig = {
  host: process.env['ISUCON_DB_HOST'] ?? '127.0.0.1',
  port: Number(process.env['ISUCON_DB_PORT'] ?? 3306),
  user: process.env['ISUCON_DB_USER'] ?? 'isucon',
  password: process.env['ISUCON_DB_PASSWORD'] ?? 'isucon',
  database: process.env['ISUCON_DB_NAME'] ?? 'isucon_listen80',
}

const adminDB = mysql.createPool(dbConfig)

function getEnv(key: string, defaultValue: string): string {
  const val = process.env[key]
  if (val !== undefined) {
    return val
  }

  return defaultValue
}

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

// レスポンス型定義
type TenantsAddResult = {
  tenant: TenantWithBilling
}

type InitializeResult = {
  lang: string
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

const app = express()
app.use(express.json())
app.use(cookieParser())
app.use(bodyParser.urlencoded({ extended: false }));
// app.use((_req: Request, res: Response, next: NextFunction) => {
//   res.set('Cache-Control', 'private')
//   next()
// })


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

  const keyFilename = getEnv('ISUCON_JWT_KEY_FILE', './public.pem')
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
  // TODO ErrNoRows
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

async function retrieveTenantRowFromHeader(req: Request): Promise<TenantRow> {
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
    throw new Error(`failed to Select tenant: name=${tenantName}`)
  }

  return tenant
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
      throw new ErrorWithStatus(403, `invalid tenant name: ${name}` )
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

    const tenant: TenantWithBilling = {
      id: id.toString(),
      name,
      display_name,
      billing: 0,
    }

    res.status(200).json({
      status: true,
      data: {
        tenant,
      }
    })
  } catch (error: any) {
    if (error.status) {
      throw error // rethrow
    }
    throw new ErrorWithStatus(500, error.toString())
  }
}))

function validateTenantName(name: string): boolean {
  if (name.match(tenantNameRegexp)) {
    return true
  }
  return false
}

async function createTenantDB(id: number): Promise<Error | undefined> {
  // テナントDBを新規に作成する
  const p = tenantDBPath(id)

  try {
    await exec(['sh', '-c', `sqlite3 ${p} < ${tenantDBSchemaFilePath}`].join(' '))
  } catch (error: any) {
    return new Error(`failed to exec sqlite3 ${p} < ${tenantDBSchemaFilePath}, out=${error.stderr}`)
  }
}

function tenantDBPath(id: number): string {
  const tenantDBDir = getEnv("ISUCON_TENANT_DB_DIR", "../tenant_db")
	return path.join(tenantDBDir, `${id.toString()}.db`)
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
        console.log(error)
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
  const closeLock = await flockByTenantID(tenantId)
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
    await closeLock()
  }
}

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

function lockFilePath(tenantId: number): string {
  const tenantDBDir = getEnv('ISUCON_TENANT_DB_DIR', '../tenant_db')
  return path.join(tenantDBDir, `${tenantId}.lock)`)
}

// テナント管理者向けAPI - 参加者追加、一覧、失格
app.get('/api/organizer/players', wrap(async (req: Request, res: Response) => {
}))
app.post('/api/organizer/players/add', wrap(async (req: Request, res: Response) => {
}))
app.post('/api/organizer/player/:playerId/disqualified', wrap(async (req: Request, res: Response) => {
}))

	// テナント管理者向けAPI - 大会管理
app.post('/api/organizer/competitions/add', wrap(async (req: Request, res: Response) => {
}))
app.post('/api/organizer/competition/:competitionId/finish', wrap(async (req: Request, res: Response) => {
}))
app.post('/api/organizer/competition/:competitionId/score', wrap(async (req: Request, res: Response) => {
}))
app.get('/api/organizer/billing', wrap(async (req: Request, res: Response) => {
}))
app.get('/api/organizer/competitions', wrap(async (req: Request, res: Response) => {
}))

	// 参加者向けAPI
app.get('/api/player/player/:playerId', wrap(async (req: Request, res: Response) => {
}))
app.get('/api/player/competition/:competitionId/ranking', wrap(async (req: Request, res: Response) => {
}))
app.get('/api/player/competitions', wrap(async (req: Request, res: Response) => {
}))


app.get('/api/me', wrap(async (req: Request, res: Response) => {
}))

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

app.use((err: ErrorWithStatus, req: Request, res: Response, next: NextFunction) => {
  console.error('error occurred: status=' + err.status + ' reason=' + err.message)
  res.status(err.status).json({
    status: false,
  })
})

const port = getEnv('SERVER_APP_PORT', '3000')
console.log('starting isuports server on :' + port + ' ...')
app.listen(port)

