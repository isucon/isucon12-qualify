<?php

namespace App\Isuports;

use JsonSerializable;

final class SuccessResult implements JsonSerializable
{
    public function __construct(
        private bool $success,
        private JsonSerializable|array|null $data = null,
    ) {
    }

    /**
     * @return array{status: bool, data?: JsonSerializable|array}
     */
    public function jsonSerialize(): array
    {
        $forJson = ['status' => $this->success];

        if (!is_null($this->data)) {
            $forJson['data'] = $this->data;
        }

        return $forJson;
    }
}

final class FailureResult implements JsonSerializable
{
    public function __construct(
        private bool $success,
        private string $message = '',
    ) {
    }

    /**
     * @return array{status: bool, message: string}
     */
    public function jsonSerialize(): array
    {
        return [
            'status' => $this->success,
            'message' => $this->message,
        ];
    }
}

final class TenantDetail implements JsonSerializable
{
    public function __construct(
        private string $name,
        private string $displayName,
    ) {
    }

    /**
     * @return array{name: string, display_name: string}
     */
    public function jsonSerialize(): array
    {
        return [
            'name' => $this->name,
            'display_name' => $this->displayName,
        ];
    }
}

final class PlayerDetail implements JsonSerializable
{
    public function __construct(
        private string $id,
        private string $displayName,
        private bool $isDisqualified,
    ) {
    }

    /**
     * @return array{id: string, display_name: string, is_disqualified: bool}
     */
    public function jsonSerialize(): array
    {
        return [
            'id' => $this->id,
            'display_name' => $this->displayName,
            'is_disqualified' => $this->isDisqualified,
        ];
    }
}

final class Role implements JsonSerializable
{
    private const NONE = 'none';
    private const ADMIN = 'admin';
    private const ORGANIZER = 'organizer';
    private const PLAYER = 'player';

    private function __construct(
        private string $str,
    ) {
    }

    public static function ofNone(): self
    {
        return new self(self::NONE);
    }

    public static function ofAdmin(): self
    {
        return new self(self::ADMIN);
    }

    public static function ofOrganizer(): self
    {
        return new self(self::ORGANIZER);
    }

    public static function ofPlayer(): self
    {
        return new self(self::PLAYER);
    }

    public function jsonSerialize(): string
    {
        return $this->str;
    }
}

final class MeHandlerResult implements JsonSerializable
{
    public function __construct(
        private TenantDetail $tenant,
        private ?PlayerDetail $me,
        private Role $role,
        private bool $loggedIn,
    ) {
    }

    /**
     * @return array{tenant: TenantDetail, me: ?PlayerDetail, role: Role, logged_in: bool}
     */
    public function jsonSerialize(): array
    {
        return [
            'tenant' => $this->tenant,
            'me' => $this->me,
            'role' => $this->role,
            'logged_in' => $this->loggedIn,
        ];
    }
}
