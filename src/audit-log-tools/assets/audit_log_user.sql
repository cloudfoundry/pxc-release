CREATE TABLE IF NOT EXISTS mysql.audit_log_user
(
    username   VARCHAR(32)  NOT NULL,
    userhost   VARCHAR(255) NOT NULL,
    filtername VARCHAR(255) NOT NULL,
    PRIMARY KEY (username, userhost),
    FOREIGN KEY `filter_name` (filtername) REFERENCES mysql.audit_log_filter (name)
) ENGINE = InnoDB CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_as_ci