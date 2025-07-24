CREATE TABLE IF NOT EXISTS mysql.audit_log_filter
(
    filter_id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    name      VARCHAR(255) NOT NULL,
    filter    JSON         NOT NULL,
    PRIMARY KEY (`filter_id`),
    UNIQUE KEY `filter_name` (`name`)
) ENGINE = InnoDB CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_as_ci