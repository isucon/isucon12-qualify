package Isuports::SQLiteTracer;
use v5.36;

use DBIx::Tracer;
use POSIX qw(strftime);
use Fcntl qw(O_CREAT O_APPEND O_WRONLY);
use JSON::PP;

sub new($class, %args) {
    my $file = $args{file} or die 'required log file path';

    my $order = { time => 1, statement => 2, args => 3, query_time => 4, dbname => 5 };
    my $encoder = JSON::PP->new->sort_by(sub {
        $order->{$JSON::PP::a} <=> $order->{$JSON::PP::b}
    });

    my $tracer = DBIx::Tracer->new(sub (%args) {
        my ($dbh, $query_time, $statement, $bind_params) = %args->@{qw/dbh time sql bind_params/};

        unless ($dbh->{Driver}{Name} eq 'SQLite') {
            return;
        }

        my $time = strftime("%Y-%m-%dT%H:%M:%S%Z", localtime(time));
        my $dbname = $dbh->{Name};

        my $json = $encoder->encode({
            time       => $time,
            statement  => $statement,
            args       => $bind_params,
            query_time => sprintf('%.6f', $query_time),
            dbname     => $dbname,
        });

        sysopen(my $fh, $file, O_CREAT|O_APPEND|O_WRONLY)
            or die sprintf("cannot trace log file: %s, %s", $file, $!);

        say $fh $json;
    });

    return bless {
        file    => $file,
        tracer  => $tracer,
    } => $class;
}

1;
