use File::Basename;
use Plack::Builder;
use Isuports::Web;

my $root_dir = File::Basename::dirname(__FILE__);

my $app = Isuports::Web->psgi($root_dir);

builder {
    enable 'ReverseProxy';
    $app;
}
