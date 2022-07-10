<?php

declare(strict_types=1);

use App\Application\Settings\SettingsInterface;
use SQLTrace\Middleware as SQLTraceMiddleware;
use DI\ContainerBuilder;
use Doctrine\DBAL\Configuration as DBConfiguration;
use Doctrine\DBAL\Connection;
use Doctrine\DBAL\DriverManager;
use Monolog\Formatter\LineFormatter;
use Monolog\Handler\StreamHandler;
use Monolog\Logger;
use Monolog\Processor\UidProcessor;
use Psr\Container\ContainerInterface;
use Psr\Log\LoggerInterface;

return function (ContainerBuilder $containerBuilder) {
    $containerBuilder->addDefinitions([
        LoggerInterface::class => function (ContainerInterface $c) {
            $settings = $c->get(SettingsInterface::class);

            $loggerSettings = $settings->get('logger');
            $logger = new Logger($loggerSettings['name']);

            $processor = new UidProcessor();
            $logger->pushProcessor($processor);

            $handler = new StreamHandler($loggerSettings['path'], $loggerSettings['level']);
            $logger->pushHandler($handler);

            return $logger;
        },
        Connection::class => function (ContainerInterface $c) {
            $databaseSettings = $c->get(SettingsInterface::class)->get('database');

            $connectionParams = [
                'dbname' => $databaseSettings['database'],
                'user' => $databaseSettings['user'],
                'password' => $databaseSettings['password'],
                'host' => $databaseSettings['host'],
                'driver' => 'pdo_mysql',
                'driverOptions' => [
                    PDO::ATTR_PERSISTENT => true,
                    PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION,
                    PDO::ATTR_DEFAULT_FETCH_MODE => PDO::FETCH_ASSOC,
                ],
            ];

            return DriverManager::getConnection($connectionParams);
        },
        DBConfiguration::class => function (ContainerInterface $c) {
            $configuration = new DBConfiguration();

            // sqliteのクエリログを出力する設定
            // sqltrace を参照
            $sqliteTraceSettings = $c->get(SettingsInterface::class)->get('sqliteTrace');
            $traceFilePath = $sqliteTraceSettings['path'];
            if ($traceFilePath) {
                $logger = new Logger($sqliteTraceSettings['name']);

                $handler = new StreamHandler($traceFilePath);
                $handler->setFormatter(new LineFormatter('%message%' . PHP_EOL));

                $logger->pushHandler($handler);

                $configuration->setMiddlewares([new SQLTraceMiddleware($logger)]);
            }

            return $configuration;
        },
    ]);
};
