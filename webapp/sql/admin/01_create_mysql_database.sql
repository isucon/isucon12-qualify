CREATE DATABASE IF NOT EXISTS `isuports`;
CREATE USER isucon IDENTIFIED BY 'isucon';
GRANT ALL PRIVILEGES ON isuports.* TO 'isucon'@'%';

SET PERSIST local_infile=1;