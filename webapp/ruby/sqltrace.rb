require 'sqlite3'
require 'time'
require 'json'

module Isuports
  module SQLite3TraceLog
    class << self
      def open(path)
        @trace_file = File.open(path, 'a', 0600)
      end

      def opened?
        !@trace_file.nil?
      end

      def write(log)
        @trace_file.puts(JSON.dump(log))
      end
    end
  end

  class SQLite3DatabaseWithTrace < SQLite3::Database
    def execute(sql, bind_vars = [], *args, &block)
      unless SQLite3TraceLog.opened?
        return super
      end

      start = Time.now
      result =
        begin
          super
        rescue => e
        end
      finish = Time.now
      query_time = finish - start

      affected_rows =
        if result
          self.changes
        end
      affected_rows ||= 0
      log = {
        time: start.iso8601,
        statement: sql,
        args: bind_vars,
        query_time:,
        affected_rows:,
      }
      SQLite3TraceLog.write(log)
      if e
        raise e
      else
        result
      end
    end
  end
end
