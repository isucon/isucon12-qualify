<?php

declare(strict_types=1);

use App\Isuports\Handlers;
use Psr\Http\Message\ResponseInterface as Response;
use Psr\Http\Message\ServerRequestInterface as Request;
use Slim\App;

return function (App $app) {
    $app->options('/{routes:.*}', function (Request $request, Response $response) {
        // CORS Pre-Flight OPTIONS Request Handler
        return $response;
    });

    // SaaS管理者向けAPI
    $app->post('/api/admin/tenants/add', Handlers::class . ':tenantsAddHandler');
    $app->get('/api/admin/tenants/billing', Handlers::class . ':tenantsBillingHandler');

    // テナント管理者向けAPI - 参加者追加、一覧、失格
    $app->get('/api/organizer/players', Handlers::class . ':playersListHandler');
    $app->post('/api/organizer/players/add', Handlers::class . ':playersAddHandler');
    $app->post('/api/organizer/player/{player_id}/disqualified', Handlers::class . ':playerDisqualifiedHandler');

    // テナント管理者向けAPI - 大会管理
    $app->post('/api/organizer/competitions/add', Handlers::class . ':competitionsAddHandler');
    $app->post('/api/organizer/competition/{competition_id}/finish', Handlers::class . ':competitionFinishHandler');
    $app->post('/api/organizer/competition/{competition_id}/score', Handlers::class . ':competitionScoreHandler');
    $app->get('/api/organizer/billing', Handlers::class . ':billingHandler');
    $app->get('/api/organizer/competitions', Handlers::class . ':organizerCompetitionsHandler');

    // 参加者向けAPI
    $app->get('/api/player/player/{player_id}', Handlers::class . ':playerHandler');
    $app->get('/api/player/competition/{competition_id}/ranking', Handlers::class . ':competitionRankingHandler');
    $app->get('/api/player/competitions', Handlers::class . ':playerCompetitionsHandler');

    // 全ロール及び未認証でも使えるhandler
    $app->get('/api/me', Handlers::class . ':meHandler');

    // ベンチマーカー向けAPI
    $app->post('/initialize', Handlers::class . ':initializeHandler');
};
