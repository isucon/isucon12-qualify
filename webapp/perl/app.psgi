use File::Basename;
use Plack::Builder;
use Isuports::Web;
use Isuports::SetCacheControlPrivateMiddleware;

my $root_dir = File::Basename::dirname(__FILE__);

my $app = Isuports::Web->psgi($root_dir);

builder {
    enable 'ReverseProxy';

    # 全APIにCache-Control: privateを設定する
    enable '+Isuports::SetCacheControlPrivateMiddleware';

    $app;
}
