# frozen_string_literal: true

require 'csv'
require 'jwt'
require 'mysql2'
require 'mysql2-cs-bind'
require 'open3'
require 'openssl'
require 'set'
require 'sinatra/base'
require 'sinatra/cookies'
require 'sinatra/json'
require 'sqlite3'

require_relative 'sqltrace'

module Isuports
  class App < Sinatra::Base
    enable :logging
    set :show_exceptions, :after_handler
    configure :development do
      require 'sinatra/reloader'
      register Sinatra::Reloader
    end
    helpers Sinatra::Cookies

    before do
      cache_control :private
    end

    TENANT_DB_SCHEMA_FILE_PATH = '../sql/tenant/10_schema.sql'
    INITIALIZE_SCRIPT = '../sql/init.sh'
    COOKIE_NAME = 'isuports_session'

    ROLE_ADMIN = 'admin'
    ROLE_ORGANIZER = 'organizer'
    ROLE_PLAYER = 'player'
    ROLE_NONE = 'none'

    # 正しいテナント名の正規表現
    TENANT_NAME_REGEXP = /^[a-z][a-z0-9-]{0,61}[a-z0-9]$/

    # アクセスしてきた人の情報
    Viewer = Struct.new(:role, :player_id, :tenant_name, :tenant_id, keyword_init: true)

    TenantRow = Struct.new(:id, :name, :display_name, :created_at, :updated_at, keyword_init: true)
    PlayerRow = Struct.new(:tenant_id, :id, :display_name, :is_disqualified, :created_at, :updated_at, keyword_init: true)
    CompetitionRow = Struct.new(:tenant_id, :id, :title, :finished_at, :created_at, :updated_at, keyword_init: true)
    PlayerScoreRow = Struct.new(:tenant_id, :id, :player_id, :competition_id, :score, :row_num, :created_at, :updated_at, keyword_init: true)

    class HttpError < StandardError
      attr_reader :code

      def initialize(code, message)
        super(message)
        @code = code
      end
    end

    def initialize(*, **)
      super
      @trace_file_path = ENV.fetch('ISUCON_SQLITE_TRACE_FILE', '')
      unless @trace_file_path.empty?
        SQLite3TraceLog.open(@trace_file_path)
      end
    end

    # エラー処理
    error HttpError do
      e = env['sinatra.error']
      logger.error("error at #{request.path}: #{e.message}")
      content_type :json
      status e.code
      JSON.dump(status: false)
    end

    helpers do
      # 管理用DBに接続する
      def connect_admin_db
        host = ENV.fetch('ISUCON_DB_HOST', '127.0.0.1')
        port = ENV.fetch('ISUCON_DB_PORT', '3306')
        username = ENV.fetch('ISUCON_DB_USER', 'isucon')
        password = ENV.fetch('ISUCON_DB_PASSWORD', 'isucon')
        database = ENV.fetch('ISUCON_DB_NAME', 'isuports')
        Mysql2::Client.new(
          host:,
          port:,
          username:,
          password:,
          database:,
          charset: 'utf8mb4',
          database_timezone: :utc,
          cast_booleans: true,
          symbolize_keys: true,
          reconnect: true,
        )
      end

      def admin_db
        Thread.current[:admin_db] ||= connect_admin_db
      end

      # テナントDBのパスを返す
      def tenant_db_path(id)
        tenant_db_dir = ENV.fetch('ISUCON_TENANT_DB_DIR', '../tenant_db')
        File.join(tenant_db_dir, "#{id}.db")
      end

      # テナントDBに接続する
      def connect_to_tenant_db(id, &block)
        path = tenant_db_path(id)
        ret = nil
        database_klass =
          if @trace_file_path.empty?
            SQLite3::Database
          else
            SQLite3DatabaseWithTrace
          end
        database_klass.new(path, results_as_hash: true) do |db|
          db.busy_timeout = 5000
          ret = block.call(db)
        end
        ret
      end

      # テナントDBを新規に作成する
      def create_tenant_db(id)
        path = tenant_db_path(id)
        out, status = Open3.capture2e('sh', '-c', "sqlite3 #{path} < #{TENANT_DB_SCHEMA_FILE_PATH}")
        unless status.success?
          raise "failed to exec sqlite3 #{path} < #{TENANT_DB_SCHEMA_FILE_PATH}, out=#{out}"
        end
        nil
      end

      # システム全体で一意なIDを生成する
      def dispense_id
        last_exception = nil
        100.times do |i|
          begin
            admin_db.xquery('REPLACE INTO id_generator (stub) VALUES (?)', 'a')
          rescue Mysql2::Error => e
            if e.error_number == 1213 # deadlock
              last_exception = e
              next
            else
              raise e
            end
          end
          return admin_db.last_id.to_s(16)
        end
        raise last_exception
      end

      # リクエストヘッダをパースしてViewerを返す
      def parse_viewer
        token_str = cookies[COOKIE_NAME]
        unless token_str
          raise HttpError.new(401, "cookie #{COOKIE_NAME} is not found")
        end

        key_filename = ENV.fetch('ISUCON_JWT_KEY_FILE', '../public.pem')
        key_src = File.read(key_filename)
        key = OpenSSL::PKey::RSA.new(key_src)
        token, _ = JWT.decode(token_str, key, true, { algorithm: 'RS256' })
        unless token.key?('sub')
          raise HttpError.new(401, "invalid token: subject is not found in token: #{token_str}")
        end

        unless token.key?('role')
          raise HttpError.new(401, "invalid token: role is not found: #{token_str}")
        end
        role = token.fetch('role')
        unless [ROLE_ADMIN, ROLE_ORGANIZER, ROLE_PLAYER].include?(role)
          raise HttpError.new(401, "invalid token: invalid role: #{token_str}")
        end

        # aud は1要素でテナント名がはいっている
        aud = token['aud']
        if !aud.is_a?(Array) || aud.size != 1
          raise HttpError.new(401, "invalid token: aud field is few or too much: #{token_str}")
        end
        tenant = retrieve_tenant_row_from_header
        if tenant.name == 'admin' && role != ROLE_ADMIN
          raise HttpError.new(401, 'tenant not found')
        end

        if tenant.name != aud[0]
          raise HttpError.new(401, "invalid token: tenant name is not match with #{request.host_with_port}: #{token_str}")
        end
        Viewer.new(
          role:,
          player_id: token.fetch('sub'),
          tenant_name: tenant.name,
          tenant_id: tenant.id,
        )
      rescue JWT::DecodeError => e
        raise HttpError.new(401, "#{e.class}: #{e.message}")
      end

      def retrieve_tenant_row_from_header
        # JWTに入っているテナント名とHostヘッダのテナント名が一致しているか確認
        base_host = ENV.fetch('ISUCON_BASE_HOSTNAME', '.t.isucon.dev')
        tenant_name = request.host_with_port.delete_suffix(base_host)

        # SaaS管理者用ドメイン
        if tenant_name == 'admin'
          return TenantRow.new(name: 'admin', display_name: 'admin')
        end

        # テナントの存在確認
        tenant = admin_db.xquery('SELECT * FROM tenant WHERE name = ?', tenant_name).first
        unless tenant
          raise HttpError.new(401, 'tenant not found')
        end
        TenantRow.new(tenant)
      end

      # 参加者を取得する
      def retrieve_player(tenant_db, id)
        row = tenant_db.get_first_row('SELECT * FROM player WHERE id = ?', [id])
        if row
          PlayerRow.new(row).tap do |player|
            player.is_disqualified = player.is_disqualified != 0
          end
        else
          nil
        end
      end

      # 参加者を認可する
      # 参加者向けAPIで呼ばれる
      def authorize_player!(tenant_db, id)
        player = retrieve_player(tenant_db, id)
        unless player
          raise HttpError.new(401, 'player not found')
        end
        if player.is_disqualified
          raise HttpError.new(403, 'player is disqualified')
        end
        nil
      end

      # 大会を取得する
      def retrieve_competition(tenant_db, id)
        row = tenant_db.get_first_row('SELECT * FROM competition WHERE id = ?', [id])
        if row
          CompetitionRow.new(row)
        else
          nil
        end
      end

      # 排他ロックのためのファイル名を生成する
      def lock_file_path(id)
        tenant_db_dir = ENV.fetch('ISUCON_TENANT_DB_DIR', '../tenant_db')
        File.join(tenant_db_dir, "#{id}.lock")
      end

      # 排他ロックする
      def flock_by_tenant_id(tenant_id, &block)
        path = lock_file_path(tenant_id)

        File.open(path, File::RDONLY | File::CREAT, 0600) do |f|
          f.flock(File::LOCK_EX)
          block.call
        end
      end

      # テナント名が規則に沿っているかチェックする
      def validate_tenant_name!(name)
        unless TENANT_NAME_REGEXP.match?(name)
          raise HttpError.new(400, "invalid tenant name: #{name}")
        end
      end

      BillingReport = Struct.new(
        :competition_id,
        :competition_title,
        :player_count,  # スコアを登録した参加者数
        :visitor_count, # ランキングを閲覧だけした(スコアを登録していない)参加者数
        :billing_player_yen,  # 請求金額 スコアを登録した参加者分
        :billing_visitor_yen, # 請求金額 ランキングを閲覧だけした(スコアを登録していない)参加者分
        :billing_yen, # 合計請求金額
        keyword_init: true,
      )

      # 大会ごとの課金レポートを計算する
      def billing_report_by_competition(tenant_db, tenant_id, competition_id)
        comp = retrieve_competition(tenant_db, competition_id)

        # ランキングにアクセスした参加者のIDを取得する
        billing_map = {}
        admin_db.xquery('SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = ? AND competition_id = ? GROUP BY player_id', tenant_id, comp.id).each do |vh|
          # competition.finished_atよりもあとの場合は、終了後に訪問したとみなして大会開催内アクセス済みとみなさない
          if comp.finished_at && comp.finished_at < vh.fetch(:min_created_at)
            next
          end
          billing_map[vh.fetch(:player_id)] = 'visitor'
        end

        # player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
        flock_by_tenant_id(tenant_id) do
          # スコアを登録した参加者のIDを取得する
          tenant_db.execute('SELECT DISTINCT(player_id) FROM player_score WHERE tenant_id = ? AND competition_id = ?', [tenant_id, comp.id]) do |row|
            pid = row.fetch('player_id')
            # スコアが登録されている参加者
            billing_map[pid] = 'player'
          end

          # 大会が終了している場合のみ請求金額が確定するので計算する
          player_count = 0
          visitor_count = 0
          if comp.finished_at
            billing_map.each_value do |category|
              case category
              when 'player'
                player_count += 1
              when 'visitor'
                visitor_count += 1
              end
            end
          end

          BillingReport.new(
            competition_id: comp.id,
            competition_title: comp.title,
            player_count:,
            visitor_count:,
            billing_player_yen: 100 * player_count, # スコアを登録した参加者は100円
            billing_visitor_yen: 10 * visitor_count,  # ランキングを閲覧だけした(スコアを登録していない)参加者は10円
            billing_yen: 100 * player_count + 10 * visitor_count,
          )
        end
      end

      def competitions_handler(v, tenant_db)
        competitions = []
        tenant_db.execute('SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC', [v.tenant_id]) do |row|
          comp = CompetitionRow.new(row)
          competitions.push({
            id: comp.id,
            title: comp.title,
            is_finished: !comp.finished_at.nil?,
          })
        end
        json(
          status: true,
          data: {
            competitions:,
          },
        )
      end
    end

    # SaaS管理者向けAPI

    # テナントを追加する
    post '/api/admin/tenants/add' do
      v = parse_viewer
      if v.tenant_name != 'admin'
        # admin: SaaS管理者用の特別なテナント名
        raise HttpError.new(404, "#{v.tenant_name} has not this API")
      end
      if v.role != ROLE_ADMIN
        raise HttpError.new(403, 'admin role required')
      end

      display_name = params[:display_name]
      name = params[:name]
      validate_tenant_name!(name)

      now = Time.now.to_i
      begin
        admin_db.xquery('INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)', name, display_name, now, now)
      rescue Mysql2::Error => e
        if e.error_number == 1062 # duplicate entry
          raise HttpError.new(400, 'duplicate tenant')
        end
        raise e
      end
      id = admin_db.last_id
      # NOTE: 先にadmin_dbに書き込まれることでこのAPIの処理中に
      #       /api/admin/tenants/billingにアクセスされるとエラーになりそう
      #       ロックなどで対処したほうが良さそう
      create_tenant_db(id)
      json(
        status: true,
        data: {
          tenant: {
            id: id.to_s,
            name: name,
            display_name: display_name,
            billing: 0,
          },
        },
      )
    end

    # テナントごとの課金レポートを最大10件、テナントのid降順で取得する
    # URL引数beforeを指定した場合、指定した値よりもidが小さいテナントの課金レポートを取得する
    get '/api/admin/tenants/billing' do
      if request.host_with_port != ENV.fetch('ISUCON_ADMIN_HOSTNAME', 'admin.t.isucon.dev')
        raise HttpError.new(404, "invalid hostname #{request.host_with_port}")
      end

      v = parse_viewer
      if v.role != ROLE_ADMIN
        raise HttpError.new(403, 'admin role required')
      end

      before = params[:before]
      before_id =
        if before
          Integer(before, 10)
        else
          nil
        end

      # テナントごとに
      #   大会ごとに
      #     scoreが登録されているplayer * 100
      #     scoreが登録されていないplayerでアクセスした人 * 10
      #   を合計したものを
      # テナントの課金とする
      tenant_billings = []
      admin_db.xquery('SELECT * FROM tenant ORDER BY id DESC').each do |row|
        t = TenantRow.new(row)
        if before_id && before_id <= t.id
          next
        end
        billing_yen = 0
        connect_to_tenant_db(t.id) do |tenant_db|
          tenant_db.execute('SELECT * FROM competition WHERE tenant_id=?', [t.id]) do |row|
            comp = CompetitionRow.new(row)
            report = billing_report_by_competition(tenant_db, t.id, comp.id)
            billing_yen += report.billing_yen
          end
        end
        tenant_billings.push({
          id: t.id.to_s,
          name: t.name,
          display_name: t.display_name,
          billing: billing_yen,
        })
        if tenant_billings.size >= 10
          break
        end
      end
      json(
        status: true,
        data: {
          tenants: tenant_billings,
        },
      )
    end

    # テナント管理者向けAPI - 参加者追加、一覧、失格

    # 参加者一覧を返す
    get '/api/organizer/players' do
      v = parse_viewer
      if v.role != ROLE_ORGANIZER
        raise HttpError.new(403, 'role organizer required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        players = []
        tenant_db.execute('SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC', [v.tenant_id]) do |row|
          player = PlayerRow.new(row)
          player.is_disqualified = player.is_disqualified != 0
          players.push(player.to_h.slice(:id, :display_name, :is_disqualified))
        end

        json(
          status: true,
          data: {
            players:,
          },
        )
      end
    end

    # テナントに参加者を追加する
    post '/api/organizer/players/add' do
      v = parse_viewer
      if v.role != ROLE_ORGANIZER
        raise HttpError.new(403, 'role organizer required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        display_names = params[:display_name]

        players = display_names.map do |display_name|
          id = dispense_id

          now = Time.now.to_i
          tenant_db.execute('INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)', [id, v.tenant_id, display_name, 0, now, now])
          player = retrieve_player(tenant_db, id)
          player.to_h.slice(:id, :display_name, :is_disqualified)
        end

        json(
          status: true,
          data: {
            players:,
          },
        )
      end
    end

    # 参加者を失格にする
    post '/api/organizer/player/:player_id/disqualified' do
      v = parse_viewer
      if v.role != ROLE_ORGANIZER
        raise HttpError.new(403, 'role organizer required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        player_id = params[:player_id]

        now = Time.now.to_i
        tenant_db.execute('UPDATE player SET is_disqualified = ?, updated_at = ? WHERE id = ?', [1, now, player_id])
        player = retrieve_player(tenant_db, player_id)
        unless player
          # 存在しないプレイヤー
          raise HttpError.new(404, 'player not found')
        end

        json(
          status: true,
          data: {
            player: player.to_h.slice(:id, :display_name, :is_disqualified),
          },
        )
      end
    end

    # テナント管理者向けAPI - 大会管理

    # 大会を追加する
    post '/api/organizer/competitions/add' do
      v = parse_viewer
      if v.role != ROLE_ORGANIZER
        raise HttpError.new(403, 'role organizer required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        title = params[:title]

        now = Time.now.to_i
        id = dispense_id
        tenant_db.execute('INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)', [id, v.tenant_id, title, nil, now, now])

        json(
          status: true,
          data: {
            competition: {
              id:,
              title:,
              is_finished: false,
            },
          },
        )
      end
    end

    # 大会を終了する
    post '/api/organizer/competition/:competition_id/finish' do
      v = parse_viewer
      if v.role != ROLE_ORGANIZER
        raise HttpError.new(403, 'role organizer required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        id = params[:competition_id]
        unless retrieve_competition(tenant_db, id)
          # 存在しない大会
          raise HttpError.new(404, 'competition not found')
        end

        now = Time.now.to_i
        tenant_db.execute('UPDATE competition SET finished_at = ?, updated_at = ? WHERE id = ?', [now, now, id])
        json(
          status: true,
        )
      end
    end

    # 大会のスコアをCSVでアップロードする
    post '/api/organizer/competition/:competition_id/score' do
      v = parse_viewer
      if v.role != ROLE_ORGANIZER
        raise HttpError.new(403, 'role organizer required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        competition_id = params[:competition_id]
        comp = retrieve_competition(tenant_db, competition_id)
        unless comp
          # 存在しない大会
          raise HttpError.new(404, 'competition not found')
        end
        if comp.finished_at
          status 400
          return json(
            status: false,
            message: 'competition is finished',
          )
        end

        csv_file = params[:scores][:tempfile]
        csv_file.set_encoding(Encoding::UTF_8)
        csv = CSV.new(csv_file, headers: true, return_headers: true)
        csv.readline
        if csv.headers != ['player_id', 'score']
          raise HttpError.new(400, 'invalid CSV headers')
        end

        # DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
        flock_by_tenant_id(v.tenant_id) do
          player_score_rows = csv.map.with_index do |row, row_num|
            if row.size != 2
              raise "row must have two columns: #{row}"
            end
            player_id, score_str = *row.values_at('player_id', 'score')
            unless retrieve_player(tenant_db, player_id)
              # 存在しない参加者が含まれている
              raise HttpError.new(400, "player not found: #{player_id}")
            end
            score = Integer(score_str, 10)
            id = dispense_id
            now = Time.now.to_i
            PlayerScoreRow.new(
              id:,
              tenant_id: v.tenant_id,
              player_id:,
              competition_id:,
              score:,
              row_num:,
              created_at: now,
              updated_at: now,
            )
          end

          tenant_db.execute('DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?', [v.tenant_id, competition_id])
          player_score_rows.each do |ps|
            tenant_db.execute('INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_num, created_at, updated_at) VALUES (:id, :tenant_id, :player_id, :competition_id, :score, :row_num, :created_at, :updated_at)', ps.to_h)
          end

          json(
            status: true,
            data: {
              rows: player_score_rows.size,
            },
          )
        end
      end
    end

    # テナント内の課金レポートを取得する
    get '/api/organizer/billing' do
      v = parse_viewer
      if v.role != ROLE_ORGANIZER
        raise HttpError.new(403, 'role organizer required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        reports = []
        tenant_db.execute('SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC', [v.tenant_id]) do |row|
          comp = CompetitionRow.new(row)
          reports.push(billing_report_by_competition(tenant_db, v.tenant_id, comp.id).to_h)
        end
        json(
          status: true,
          data: {
            reports:,
          },
        )
      end
    end

    # 大会の一覧を取得する
    get '/api/organizer/competitions' do
      v = parse_viewer
      if v.role != ROLE_ORGANIZER
        raise HttpError.new(403, 'role organizer required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        competitions_handler(v, tenant_db)
      end
    end

    # 参加者向けAPI

    # 参加者の詳細情報を取得する
    get '/api/player/player/:player_id' do
      v = parse_viewer
      if v.role != ROLE_PLAYER
        raise HttpError.new(403, 'role player required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        authorize_player!(tenant_db, v.player_id)

        player_id = params[:player_id]
        player = retrieve_player(tenant_db, player_id)
        unless player
          raise HttpError.new(404, 'player not found')
        end
        competitions = tenant_db.execute('SELECT * FROM competition WHERE tenant_id = ? ORDER BY created_at ASC', [v.tenant_id]).map { |row| CompetitionRow.new(row) }
        # player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
        flock_by_tenant_id(v.tenant_id) do
          player_score_rows = competitions.filter_map do |c|
            # 最後にCSVに登場したスコアを採用する = row_numが一番大きいもの
            row = tenant_db.get_first_row('SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? AND player_id = ? ORDER BY row_num DESC LIMIT 1', [v.tenant_id, c.id, player.id])
            if row
              PlayerScoreRow.new(row)
            else
              # 行がない = スコアが記録されてない
              nil
            end
          end

          scores = player_score_rows.map do |ps|
            comp = retrieve_competition(tenant_db, ps.competition_id)
            {
              competition_title: comp.title,
              score: ps.score,
            }
          end

          json(
            status: true,
            data: {
              player: player.to_h.slice(:id, :display_name, :is_disqualified),
              scores:,
            },
          )
        end
      end
    end

    CompetitionRank = Struct.new(:rank, :score, :player_id, :player_display_name, :row_num, keyword_init: true)

    # 大会ごとのランキングを取得する
    get '/api/player/competition/:competition_id/ranking' do
      v = parse_viewer
      if v.role != ROLE_PLAYER
        raise HttpError.new(403, 'role player required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        authorize_player!(tenant_db, v.player_id)

        competition_id = params[:competition_id]

        # 大会の存在確認
        competition = retrieve_competition(tenant_db, competition_id)
        unless competition
          raise HttpError.new(404, 'competition not found')
        end

        now = Time.now.to_i
        tenant = TenantRow.new(admin_db.xquery('SELECT * FROM tenant WHERE id = ?', v.tenant_id).first)
        admin_db.xquery('INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)', v.player_id, tenant.id, competition_id, now, now)

        rank_after_str = params[:rank_after]
        rank_after =
          if rank_after_str
            Integer(rank_after_str, 10)
          else
            0
          end

        # player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
        flock_by_tenant_id(v.tenant_id) do
          ranks = []
          scored_player_set = Set.new
          tenant_db.execute('SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? ORDER BY row_num DESC', [tenant.id, competition_id]) do |row|
            ps = PlayerScoreRow.new(row)
            # player_scoreが同一player_id内ではrow_numの降順でソートされているので
            # 現れたのが2回目以降のplayer_idはより大きいrow_numでスコアが出ているとみなせる
            if scored_player_set.member?(ps.player_id)
              next
            end
            scored_player_set.add(ps.player_id)
            player = retrieve_player(tenant_db, ps.player_id)
            ranks.push(CompetitionRank.new(
              score: ps.score,
              player_id: player.id,
              player_display_name: player.display_name,
              row_num: ps.row_num,
            ))
          end
          ranks.sort! do |a, b|
            if a.score == b.score
              a.row_num <=> b.row_num
            else
              b.score <=> a.score
            end
          end
          paged_ranks = ranks.drop(rank_after).take(100).map.with_index do |rank, i|
            {
              rank: rank_after + i + 1,
              score: rank.score,
              player_id: rank.player_id,
              player_display_name: rank.player_display_name,
            }
          end

          json(
            status: true,
            data: {
              competition: {
                id: competition.id,
                title: competition.title,
                is_finished: !competition.finished_at.nil?,
              },
              ranks: paged_ranks,
            },
          )
        end
      end
    end

    # 大会の一覧を取得する
    get '/api/player/competitions' do
      v = parse_viewer
      if v.role != ROLE_PLAYER
        raise HttpError.new(403, 'role player required')
      end

      connect_to_tenant_db(v.tenant_id) do |tenant_db|
        authorize_player!(tenant_db, v.player_id)
        competitions_handler(v, tenant_db)
      end
    end

    # 全ロール及び未認証でも使えるhandler

    # JWTで認証した結果、テナントやユーザ情報を返す
    get '/api/me' do
      tenant = retrieve_tenant_row_from_header
      v =
        begin
          parse_viewer
        rescue HttpError => e
          return json(
            status: true,
            data: {
              tenant: tenant.to_h.slice(:name, :display_name),
              me: nil,
              role: ROLE_NONE,
              logged_in: false,
            },
          )
        end
      if v.role == ROLE_ADMIN|| v.role == ROLE_ORGANIZER
        json(
          status: true,
          data: {
            tenant: tenant.to_h.slice(:name, :display_name),
            me: nil,
            role: v.role,
            logged_in: true,
          },
        )
      else
        connect_to_tenant_db(v.tenant_id) do |tenant_db|
          player = retrieve_player(tenant_db, v.player_id)
          if player
            json(
              status: true,
              data: {
                tenant: tenant.to_h.slice(:name, :display_name),
                me: player.to_h.slice(:id, :display_name, :is_disqualified),
                role: v.role,
                logged_in: true,
              },
            )
          else
            json(
              status: true,
              data: {
                tenant: tenant.to_h.slice(:name, :display_name),
                me: nil,
                role: ROLE_NONE,
                logged_in: false,
              },
            )
          end
        end
      end
    end

    # ベンチマーカー向けAPI

    # ベンチマーカーが起動したときに最初に呼ぶ
    # データベースの初期化などが実行されるため、スキーマを変更した場合などは適宜改変すること
    post '/initialize' do
      out, status = Open3.capture2e(INITIALIZE_SCRIPT)
      unless status.success?
        raise HttpError.new(500, "error command execution: #{out}")
      end
      json(
        status: true,
        data: {
          lang: 'ruby',
        },
      )
    end
  end
end
