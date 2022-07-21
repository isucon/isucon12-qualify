<?php

declare(strict_types=1);

use App\Application\Settings\Settings;
use App\Application\Settings\SettingsInterface;
use DI\ContainerBuilder;
use Monolog\Logger;

return function (ContainerBuilder $containerBuilder) {

    // Global Settings Object
    $containerBuilder->addDefinitions([
        SettingsInterface::class => function () {
            return new Settings([
                'displayErrorDetails' => true, // Should be set to false in production
                'logError'            => true,
                'logErrorDetails'     => true,
                'logger' => [
                    'name' => 'isuports',
                    'path' => 'php://stdout',
                    'level' => Logger::DEBUG,
                ],
                'database' => [
                    'host' => getenv('ISUCON_DB_HOST') ?: '127.0.0.1',
                    'port' => getenv('ISUCON_DB_PORT') ?: '3306',
                    'database' => getenv('ISUCON_DB_NAME') ?: 'isuports',
                    'user' => getenv('ISUCON_DB_USER') ?: 'isucon',
                    'password' => getenv('ISUCON_DB_PASSWORD') ?: 'isucon',
                ],
                // sqliteのクエリログを出力する設定
                // 環境変数 ISUCON_SQLITE_TRACE_FILE を設定すると、そのファイルにクエリログをJSON形式で出力する
                // 未設定なら出力しない
                'sqliteTrace' => [
                    'name' => 'sqlite-trace',
                    'path' => getenv('ISUCON_SQLITE_TRACE_FILE') ?: '',
                ],
            ]);
        }
    ]);
};
