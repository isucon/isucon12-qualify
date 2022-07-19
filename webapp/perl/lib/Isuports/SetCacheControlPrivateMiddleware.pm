package Isuports::SetCacheControlPrivateMiddleware;
use v5.36;
use parent qw( Plack::Middleware );

sub call {
    my($self, $env) = @_;

    my $res = $self->app->($env);
    push $res->[1]->@* => ('Cache-Control' => 'private');

    return $res;
}

1;
