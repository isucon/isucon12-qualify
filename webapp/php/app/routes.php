<?php

declare(strict_types=1);

use App\Isuports\Handlers;
use Slim\App;

return function (App $app) {
    $app->get('/api/me', Handlers::class . ':meHandler');
};
