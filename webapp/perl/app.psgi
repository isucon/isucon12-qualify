use File::Basename;
use Plack::Builder;
use Isuports::Web;

my $root_dir = File::Basename::dirname(__FILE__);

my $app = Isuports::Web->psgi($root_dir);

builder {
    enable 'ReverseProxy';
    enable sub {
        my $app = shift;
        return sub {
            my $env = shift;
            my $res = $app->($env);
            push $res->[1]->@*, 'Cache-Control' => 'private';
            return $res;
        };
    };
    $app;
}
