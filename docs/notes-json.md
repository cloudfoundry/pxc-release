# MySQL vs. MariaDB JSON functionality

MySQL introduced JSON support[1] first. Subsequently, there were many requests asking for the same in MariaDB.   MySQL has had this in a stable release since 5.7.9(GA) (Oct 2015) and MariaDB added JSON support in a stable release as of 10.2.6(GA) (May 2017).

Most of this support in MariaDB and MySQL comes in the form of SQL functions[2] - most (but not all) of which have identical signatures and behavior across MariaDB and MySQL.  There are a few functions that do not have an exact match in MySQL:

https://mariadb.com/kb/en/library/function-differences-between-mariadb-103-and-mysql-57/#json

So, there could be existing applications that rely on MariaDB 10.2 specific functions that would need to be updated to work on MySQL 5.7 (and vice versa).  One platform is not a pure superset of the other.

MySQL 5.7 also supports a JSON data type which MariaDB does not.  This provides convenient input validation and stores in a structured binary form to speed up extraction of elements from document  encoded in a JSON column.  There are also some convenience operators[3][4] that alias the extraction functionality in a terser query format.

All the MySQL 5.7 functionality exists by default in Percona Server and PXC.

1. https://dev.mysql.com/doc/refman/5.7/en/json.html
1. https://dev.mysql.com/doc/refman/5.7/en/json-function-reference.html
1. https://dev.mysql.com/doc/refman/5.7/en/json-search-functions.html#operator_json-column-path
1. https://dev.mysql.com/doc/refman/5.7/en/json-search-functions.html#operator_json-inline-path
