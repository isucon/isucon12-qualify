import { Database, ISqlite } from 'sqlite'
import { appendFile } from 'fs'

type SQLLog = {
  time: string
  statement: string
  args: string[]
  query_time: number
  affected_rows: number
}

export const useSqliteTraceHook = (db: Database, filePath: string) => {
  // これは情報量がたりないので使わない…
  // db.on('profile', (sql: string, nsec: number, extra: any) => {
  //   console.log(sql + ', nsec=' + nsec + ' extra=' + extra)
  // })

  const queryFormat = (
    before: Date,
    after: Date,
    affectedRows: number,
    sql: ISqlite.SqlType,
    ...params: any[]
  ): SQLLog => {
    return {
      time: before.toISOString(),
      statement: sql.toString(),
      args: params,
      query_time: (after.getTime() - before.getTime()) / 1000,
      affected_rows: affectedRows,
    }
  }

  const writeFile = (log: SQLLog) => {
    const str = JSON.stringify(log, null, 2)
    appendFile(filePath, str + '\n', (error) => {
      if (error) {
        console.error(`warning: failed to write SQLite Log: ${error}`)
      }
    })
  }

  const origGet = db.get.bind(db)
  db.get = async <T = any>(sql: ISqlite.SqlType, ...params: any[]): Promise<T | undefined> => {
    const before = new Date()
    const res = await origGet(sql, ...params)
    const after = new Date()
    const log = queryFormat(before, after, 0, sql, ...params)
    writeFile(log)
    return res
  }

  const origAll = db.all.bind(db)
  db.all = async <T = any[]>(sql: ISqlite.SqlType, ...params: any[]): Promise<T> => {
    const before = new Date()
    const res: T = await origAll(sql, ...params)
    const after = new Date()
    const log = queryFormat(before, after, 0, sql, ...params)
    writeFile(log)
    return res
  }

  const origRun = db.run.bind(db)
  db.run = async (sql: ISqlite.SqlType, ...params: any[]): Promise<ISqlite.RunResult> => {
    const before = new Date()
    const res: ISqlite.RunResult = await origRun(sql, ...params)
    const after = new Date()
    const log = queryFormat(before, after, res.changes ?? 0, sql, ...params)
    writeFile(log)
    return res
  }

  return db
}
