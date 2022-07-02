<?php

declare(strict_types=1);

namespace SQLTrace;

use Doctrine\DBAL\Driver as DriverInterface;
use Doctrine\DBAL\Driver\Middleware\AbstractDriverMiddleware;
use Psr\Log\LoggerInterface;

/**
 * @see {\Doctrine\DBAL\Logging\Driver}
 */
final class Driver extends AbstractDriverMiddleware implements DriverInterface
{
    public function __construct(
        DriverInterface $driver,
        private LoggerInterface $logger,
    ) {
        parent::__construct($driver);
    }

    public function connect(array $params): Connection
    {
        return new Connection(parent::connect($params), $this->logger);
    }
}
