package thermostat_test

import (
	"database/sql"
	"strconv"

	"github.com/cloudfoundry/gosigar"
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"fmt"
	"os"
	. "thermostat"
)

var _ = Describe("Dedicated MySQL", func() {
	var config *Config
	var db *sql.DB

	BeforeEach(func() {
		var err error
		config, err = LoadConfig()
		Expect(err).ToNot(HaveOccurred())

		db, err = Db(config)
		Expect(err).ToNot(HaveOccurred())
	})
	AfterEach(func() {
		_ = db.Close()
	})

	Describe("default schema", func() {
		It("is created", func() {
			desiredSchemaName, err := config.Properties.FindString("default_schema")
			Expect(err).ToNot(HaveOccurred())

			schemaNames := []string{}

			rows, err := db.Query("SELECT schema_name FROM information_schema.schemata")
			Expect(err).ToNot(HaveOccurred())
			defer func() {
				_ = rows.Close()
			}()

			for rows.Next() {
				var schema string
				err = rows.Scan(&schema)
				Expect(err).ToNot(HaveOccurred())
				schemaNames = append(schemaNames, schema)
			}
			Expect(rows.Err()).ToNot(HaveOccurred())

			Expect(schemaNames).To(ContainElement(desiredSchemaName))
		})
	})

	DescribeTable("general options",
		func(variable, expected string) {
			value, err := DbVariableValue(db, variable)
			Expect(err).ToNot(HaveOccurred())
			Expect(value).To(Equal(expected))

		},
		Entry("turns on the event scheduler", "event_scheduler", "ON"),
		Entry("table definition cache is set to 8192", "table_definition_cache", "8192"),
		Entry("max connections is set to 750", "max_connections", "750"),
		Entry("sets myisam recovery option to force and backup", "myisam_recover_options", "BACKUP,FORCE"),
	)

	Describe("lower_case_table_names", func() {
		var lowerCaseTableNames bool
		BeforeEach(func() {
			var err error
			lowerCaseTableNames, err =
				config.Properties.FindBool("enable_lower_case_table_names")
			Expect(err).ToNot(HaveOccurred())
		})

		It("Sets lower_case_table_names to 1/0 based on true/false", func() {
			lowerCaseTableNamesStringDbValue, err := DbVariableValue(db, "lower_case_table_names")
			Expect(err).ToNot(HaveOccurred())
			lowerCaseTableNamesEnabled := false
			if lowerCaseTableNamesStringDbValue == "1" {
				lowerCaseTableNamesEnabled = true
			}

			Expect(lowerCaseTableNames).To(Equal(lowerCaseTableNamesEnabled))
		})
	})

	Describe("logging", func() {
		Context("audit logging", func() {
			var auditLoggingRequested bool
			BeforeEach(func() {
				var err error
				auditLoggingRequested, err =
					config.Properties.FindBool("audit_log.enabled")
				Expect(err).ToNot(HaveOccurred())
			})
			Context("when the audit log is enabled", func() {
				BeforeEach(func() {
					if !auditLoggingRequested {
						Skip("Audit logging is not enabled")
					}
				})
				It("loads the audit logging plugin", func() {
					auditPluginActive, err := DbPluginActive(db, "audit_log")
					Expect(err).ToNot(HaveOccurred())
					Expect(auditPluginActive).To(Equal(auditLoggingRequested))
				})
				It("does not log to syslog", func() {
					handler, err := DbVariableValue(db, "audit_log_handler")
					Expect(err).ToNot(HaveOccurred())
					Expect(handler).ToNot(Equal("SYSLOG"))
				})
				It("logs to the expected path", func() {
					path, err := DbVariableValue(db, "audit_log_file")
					Expect(err).ToNot(HaveOccurred())
					Expect(path).To(Equal("/var/vcap/sys/log/mysql/mysql_audit_log"))
				})
				It("logs in CSV format", func() {
					format, err := DbVariableValue(db, "audit_log_format")
					Expect(err).ToNot(HaveOccurred())
					Expect(format).To(Equal("CSV"))
				})
				It("rotates logs when they reach 50MB in size", func() {
					size, err := DbVariableValue(db, "audit_log_rotate_on_size")
					Expect(err).ToNot(HaveOccurred())

					rotateOnSize, err := strconv.Atoi(size)
					Expect(err).ToNot(HaveOccurred())
					Expect(rotateOnSize).To(Equal(50 * 1024 * 1024))
				})
				It("keeps the last 5 rotated logs", func() {
					rotations, err := DbVariableValue(db, "audit_log_rotations")
					Expect(err).ToNot(HaveOccurred())
					Expect(rotations).To(Equal("5"))
				})
				It("has a name that will not be rotated by the BOSH stemcell log rotation", func() {
					path, err := DbVariableValue(db, "audit_log_file")
					Expect(err).ToNot(HaveOccurred())
					Expect(path).ToNot(HaveSuffix(".log"))
				})
			})
		})

		It("logs mysql error output to the expected path ", func() {
			path, err := DbVariableValue(db, "log_error")
			Expect(err).ToNot(HaveOccurred())
			Expect(path).To(Equal("/var/vcap/sys/log/mysql/mysql.err.log"))
		})

		Context("slow query log", func() {
			It("is set", func() {
				enabledAtServer, err := DbVariableEnabled(db, "slow_query_log")
				Expect(err).ToNot(HaveOccurred())

				Expect(enabledAtServer).To(BeTrue())
			})
			It("logs to a file under /var/vcap/sys/log/", func() {
				path, err := DbVariableValue(db, "slow_query_log_file")
				Expect(err).ToNot(HaveOccurred())

				Expect(path).To(HavePrefix("/var/vcap/sys/log/"))
			})
			It("logs to a file with .log extension", func() {
				path, err := DbVariableValue(db, "slow_query_log_file")
				Expect(err).ToNot(HaveOccurred())

				Expect(path).To(HaveSuffix(".log"))
			})
		})

	})
	Describe("replication", func() {
		It("sets server-id to greater than 0", func() {
			serverId, err := DbVariableValue(db, "server_id")
			Expect(err).ToNot(HaveOccurred())

			Expect(strconv.Atoi(serverId)).To(BeNumerically(">", 0))
		})

		It("sets the database in an appropriate read-only mode", func() {
			isLeaderFollowerEnabled, err := config.Properties.FindBool("leader_follower.enabled")
			Expect(err).NotTo(HaveOccurred())

			leaderCnf := "/var/vcap/sys/run/mysql/lf-state/leader.cnf"
			_, err = os.Stat(leaderCnf)

			leaderCnfNotExists := os.IsNotExist(err)

			var expectedReadOnlyState string
			if isLeaderFollowerEnabled {
				if leaderCnfNotExists {
					expectedReadOnlyState = "ON"
				} else {
					expectedReadOnlyState = "OFF"
				}
			} else {
				expectedReadOnlyState = "OFF"
			}

			Expect(DbVariableValue(db, "read_only")).To(Equal(expectedReadOnlyState))
			Expect(DbVariableValue(db, "super_read_only")).To(Equal(expectedReadOnlyState))
		})

		Context("When semi-synchronous replication is enabled", func() {
			BeforeEach(func() {
				mode, err := config.Properties.FindString("leader_follower.replication_mode")
				Expect(err).NotTo(HaveOccurred())

				if mode != "semi-sync" {
					Skip("Replication mode is not semi-sync")
				}
			})

			It("Loads the semi-synchronous replication plugins", func() {
				semisyncMasterEnabled, err := DbPluginActive(db, "rpl_semi_sync_master")
				Expect(err).ToNot(HaveOccurred())
				Expect(semisyncMasterEnabled).To(BeTrue())

				semisyncSlaveEnabled, err := DbPluginActive(db, "rpl_semi_sync_slave")
				Expect(err).ToNot(HaveOccurred())
				Expect(semisyncSlaveEnabled).To(BeTrue())
			})

			It("Has rpl_semi_sync_slave_enabled set to ON", func() {
				value, err := DbVariableValue(db, "rpl_semi_sync_slave_enabled")
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal("ON"))
			})

			It("sets the semi sync timeout to the specified value", func() {
				dbVal, err := DbVariableValue(db, "rpl_semi_sync_master_timeout")
				Expect(err).ToNot(HaveOccurred())

				actualTimeout, err := strconv.ParseInt(dbVal, 10, 64)
				Expect(err).ToNot(HaveOccurred())

				configuredTimeout, err := config.Properties.Find("leader_follower.semi_sync_ack_timeout_in_ms")
				Expect(err).ToNot(HaveOccurred())

				Expect(actualTimeout).To(BeEquivalentTo(configuredTimeout))
			})
		})

		Context("When semi-synchronous replication is disabled", func() {
			BeforeEach(func() {
				mode, err := config.Properties.FindString("leader_follower.replication_mode")
				Expect(err).NotTo(HaveOccurred())

				if mode == "semi-sync" {
					Skip("Replication mode is semi-sync")
				}
			})

			It("Does not load the semi-synchronous replication plugins", func() {
				semisyncMasterEnabled, err := DbPluginActive(db, "rpl_semi_sync_master")
				Expect(err).ToNot(HaveOccurred())
				Expect(semisyncMasterEnabled).To(BeFalse())

				semisyncSlaveEnabled, err := DbPluginActive(db, "rpl_semi_sync_slave")
				Expect(err).ToNot(HaveOccurred())
				Expect(semisyncSlaveEnabled).To(BeFalse())
			})
		})

		DescribeTable("replication options",
			func(variable, expected string) {
				value, err := DbVariableValue(db, variable)
				Expect(err).ToNot(HaveOccurred())
				Expect(value).To(Equal(expected))
			},

			Entry("abort server when mysql cannot write to binlog", "binlog_error_action", "ABORT_SERVER"),
			Entry("bin log format set to row", "binlog_format", "ROW"),
			Entry("bin log has no delay in syncing group commits", "binlog_group_commit_sync_delay", "0"),
			Entry("binlog-row-image is set to minimal", "binlog_row_image", "MINIMAL"),
			Entry("binlog-rows-query-log-events is enabled", "binlog_rows_query_log_events", "ON"),
			Entry("binlog-stmt-cache-size is set to 4M", "binlog_stmt_cache_size", "4194304"),
			Entry("enforce-gtid-consistency is enabled", "enforce_gtid_consistency", "ON"),
			Entry("expire_logs_days is set to 3 days", "expire_logs_days", "3"),
			Entry("gtid-mode is enabled", "gtid_mode", "ON"),
			Entry("binary logs are enabled", "log_bin", "ON"),
			Entry("binary log basename is reasonable", "log_bin_basename", "/var/vcap/store/mysql/data/mysql-bin"),
			Entry("log-slave-updates is enabled", "log_slave_updates", "ON"),
			Entry("trust users to create stored functions in a non strict mode", "log_bin_trust_function_creators", "ON"),
			Entry("master-info-repository is set to TABLE", "master_info_repository", "TABLE"),
			Entry("master-verify-checksum is enabled", "master_verify_checksum", "ON"),
			Entry("bin log max cache size is set to 2G", "max_binlog_cache_size", "2147483648"),
			Entry("bin log max size set to 512M", "max_binlog_size", "536870912"),
			Entry("bin log statement max cache size is set to 2G", "max_binlog_stmt_cache_size", "2147483648"),
			Entry("relay-log is set with a reasonable basename", "relay_log", "mysql-relay"),
			Entry("relay-log-info-repository is set to TABLE", "relay_log_info_repository", "TABLE"),
			Entry("slave-sql-verify-checksum is enabled", "slave_sql_verify_checksum", "ON"),
			Entry("bin log sync transactions before being committed", "sync_binlog", "1"),
			Entry("Recover relay logs when follower dies", "relay_log_recovery", "ON"),
		)
	})

	Describe("innodb", func() {
		Context("for durability", func() {
			It("flushes logs once per second", func() {
				logFlush, err := DbVariableValue(db, "innodb_flush_log_at_timeout")
				Expect(err).ToNot(HaveOccurred())
				Expect(logFlush).To(Equal("1"))
			})

			It("flushes logs at trx commit ", func() {
				trxLogFlush, err := DbVariableValue(db, "innodb_flush_log_at_trx_commit")
				Expect(err).ToNot(HaveOccurred())
				Expect(trxLogFlush).To(Equal("1"))
			})
		})

		Context("workload settings", func() {
			var workload string
			BeforeEach(func() {
				var err error
				workload, err = config.Properties.FindString("workload")
				if err != nil {
					workload = "mixed"
				}
			})

			It("innodb_log_buffer_size greater than the default, 32M", func() {
				logBufferSize, err := DbVariableValue(db, "innodb_log_buffer_size")
				Expect(err).ToNot(HaveOccurred())
				Expect(logBufferSize).To(Equal("33554432"))
			})

			It("fails instead of warning in certain error cases", func() {
				logBufferSize, err := DbVariableValue(db, "innodb_strict_mode")
				Expect(err).ToNot(HaveOccurred())
				Expect(logBufferSize).To(Equal("ON"))
			})

			Context("for mixed-workload", func() {
				BeforeEach(func() {
					if workload != "mixed" {
						Skip("Mixed workload is not configured")
					}
				})

				It("sets max_allowed_packet to 256M", func() {
					val, err := DbVariableValue(db, "max_allowed_packet")
					Expect(err).ToNot(HaveOccurred())
					Expect(val).To(Equal("268435456"))
				})

				It("sets log file size to 256M", func() {
					val, err := DbVariableValue(db, "innodb_log_file_size")
					Expect(err).ToNot(HaveOccurred())
					Expect(val).To(Equal("268435456"))
				})

				It("sets buffer pool size to be greater than 50% of the total memory allocated to the vm", func() {
					bufferPoolSize, err := DbVariableValue(db, "innodb_buffer_pool_size")
					Expect(err).ToNot(HaveOccurred())

					bufferPoolSizeInt, err := strconv.Atoi(bufferPoolSize)
					Expect(err).ToNot(HaveOccurred())

					mem := sigar.Mem{}
					err = mem.Get()
					halfTotalMemory := mem.Total / 2

					Expect(err).ToNot(HaveOccurred())
					Expect(bufferPoolSizeInt).Should(BeNumerically(">", halfTotalMemory))
				})

			})

			Context("for read-heavy workload", func() {
				BeforeEach(func() {
					if workload != "read-heavy" {
						Skip("read-heavy workload is not configured")
					}
				})

				It("sets max_allowed_packet to 1G", func() {
					val, err := DbVariableValue(db, "max_allowed_packet")
					Expect(err).ToNot(HaveOccurred())
					Expect(val).To(Equal("1073741824"))
				})

				It("sets innodb_flush_method to O_DIRECT", func() {
					flushMethod, err := DbVariableValue(db, "innodb_flush_method")
					Expect(err).ToNot(HaveOccurred())
					Expect(flushMethod).To(Equal("O_DIRECT"))
				})

				It("sets buffer pool size to be greater than 75% of the total memory allocated to the vm", func() {
					bufferPoolSize, err := DbVariableValue(db, "innodb_buffer_pool_size")
					Expect(err).ToNot(HaveOccurred())

					bufferPoolSizeInt, err := strconv.Atoi(bufferPoolSize)
					Expect(err).ToNot(HaveOccurred())

					mem := sigar.Mem{}
					err = mem.Get()
					targetPercentage := uint64(float64(mem.Total) * 0.75)

					Expect(err).ToNot(HaveOccurred())
					Expect(bufferPoolSizeInt).Should(BeNumerically(">", targetPercentage))
				})
			})

			Context("for write-heavy workload", func() {
				BeforeEach(func() {
					if workload != "write-heavy" {
						Skip("write-heavy workload is not configured")
					}
				})

				It("sets max_allowed_packet to 1G", func() {
					val, err := DbVariableValue(db, "max_allowed_packet")
					Expect(err).ToNot(HaveOccurred())
					Expect(val).To(Equal("1073741824"))
				})

				It("sets innodb_flush_method to O_DIRECT", func() {
					flushMethod, err := DbVariableValue(db, "innodb_flush_method")
					Expect(err).ToNot(HaveOccurred())
					Expect(flushMethod).To(Equal("O_DIRECT"))
				})

				It("sets buffer pool size to be greater than 75% of the total memory allocated to the vm", func() {
					bufferPoolSize, err := DbVariableValue(db, "innodb_buffer_pool_size")
					Expect(err).ToNot(HaveOccurred())

					bufferPoolSizeInt, err := strconv.Atoi(bufferPoolSize)
					Expect(err).ToNot(HaveOccurred())

					mem := sigar.Mem{}
					err = mem.Get()
					targetPercentage := uint64(float64(mem.Total) * 0.75)

					Expect(err).ToNot(HaveOccurred())
					Expect(bufferPoolSizeInt).Should(BeNumerically(">", targetPercentage))
				})

				It("sets innodb_log_file_size to 1G", func() {
					flushMethod, err := DbVariableValue(db, "innodb_log_file_size")
					Expect(err).ToNot(HaveOccurred())
					Expect(flushMethod).To(Equal("1073741824"))
				})

			})
		})

		Context("for performance", func() {
			It("sets auto increment lock mode to interleaved", func() {
				autoIncModeVal, err := DbVariableValue(db, "innodb_autoinc_lock_mode")
				Expect(err).ToNot(HaveOccurred())
				Expect(autoIncModeVal).To(Equal("2"))
			})
		})
	})

	Describe("userstat", func() {
		var userstatRequested bool
		BeforeEach(func() {
			var err error
			userstatRequested, err =
				config.Properties.FindBool("userstat.enabled")
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when userstat is enabled", func() {
			BeforeEach(func() {
				if !userstatRequested {
					Skip("Userstat is not enabled")
				}
			})

			It("enables user stat logging", func() {
				userstatEnabled, err := DbVariableValue(db, "userstat")
				Expect(err).ToNot(HaveOccurred())
				Expect(userstatEnabled).To(Equal("ON"))
			})
		})

		Context("when userstat is not enabled", func() {
			BeforeEach(func() {
				if userstatRequested {
					Skip("Userstat is enabled")
				}
			})
			It("does not enable user stat logging", func() {
				userstatEnabled, err := DbVariableValue(db, "userstat")
				Expect(err).ToNot(HaveOccurred())
				Expect(userstatEnabled).To(Equal("OFF"))
			})
		})
	})

	Describe("tmpdir", func() {
		It("is set to /var/vcap/data/mysql/tmp", func() {
			tmpDirValue, err := DbVariableValue(db, "tmpdir")
			Expect(err).ToNot(HaveOccurred())

			Expect(tmpDirValue).To(Equal("/var/vcap/data/mysql/tmp"))
		})

		It("innodb_tmpdir remains unset so that it can inherit its value from mysqld tmpdir config", func() {
			tmpDirValue, err := DbVariableValue(db, "innodb_tmpdir")
			Expect(err).ToNot(HaveOccurred())

			Expect(tmpDirValue).To(Equal(""))
		})
	})

	Describe("character set configurations", func() {
		It("character set server is set to the specified character set", func() {
			characterSetServer, err := DbVariableValue(db, "character_set_server")
			Expect(err).ToNot(HaveOccurred())

			Expect(characterSetServer).To(Equal(config.Properties["default_char_set"]))
		})

		It("default schema's character set is configured to the specified character set", func() {
			schemaName, err := config.Properties.FindString("default_schema")
			Expect(err).ToNot(HaveOccurred())

			charSetName, ok := config.Properties["default_char_set"].(string)

			if ok {
				var schemaCharset string
				err = DbExecuteQuery(db,
					fmt.Sprintf("SELECT default_character_set_name FROM information_schema.SCHEMATA WHERE schema_name = '%s'",
						schemaName),
					&schemaCharset)
				Expect(err).ToNot(HaveOccurred())

				Expect(schemaCharset).To(Equal(charSetName))
			}
		})
	})

	Describe("collation configurations", func() {
		BeforeEach(func() {
			if value, ok := config.Properties["default_collation"]; !ok || (value == nil) {
				Skip("default_collation is not configured")
			}
		})

		It("collation server is set to the specified collation", func() {
			collationServer, err := DbVariableValue(db, "collation_server")
			Expect(err).ToNot(HaveOccurred())

			Expect(collationServer).To(Equal(config.Properties["default_collation"]))
		})

		It("default schema's character set is configured to the specified character set", func() {
			schemaName, err := config.Properties.FindString("default_schema")
			Expect(err).ToNot(HaveOccurred())

			charSetName, ok := config.Properties["default_collation"].(string)

			if ok {
				var schemaCollation string
				err = DbExecuteQuery(db,
					fmt.Sprintf("SELECT default_collation_name FROM information_schema.SCHEMATA WHERE schema_name = '%s'",
						schemaName),
					&schemaCollation)
				Expect(err).ToNot(HaveOccurred())

				Expect(schemaCollation).To(Equal(charSetName))
			}
		})
	})

	Describe("security", func() {
		Describe("skip-name-resolve", func() {
			It("is enabled to prevent a known buffer overflow", func() {
				skipNameResolve, err := DbVariableValue(db, "skip_name_resolve")
				Expect(err).ToNot(HaveOccurred())
				Expect(skipNameResolve).To(Equal("ON"))
			})
		})

		Describe("skip-symbolic-links", func() {
			It("is enabled to prevent undesired file system access by users", func() {
				symlinks, err := DbVariableValue(db, "have_symlink")
				Expect(err).ToNot(HaveOccurred())
				Expect(symlinks).To(Equal("DISABLED"))
			})
		})

		Describe("ssl enabled", func() {
			It("is enabled have_ssl to allow ssl communication", func() {
				haveSSL, err := DbVariableValue(db, "have_ssl")
				Expect(err).ToNot(HaveOccurred())
				Expect(haveSSL).To(Equal("YES"))
			})
			It("is enabled have_openssl to allow ssl communication", func() {
				haveSSL, err := DbVariableValue(db, "have_openssl")
				Expect(err).ToNot(HaveOccurred())
				Expect(haveSSL).To(Equal("YES"))
			})
		})

		Describe("enforce_client_tls", func() {
			It("sets require-secure-transport appropriately", func() {
				enforceClientTLS, err := config.Properties.FindBool("enforce_client_tls")
				Expect(err).ToNot(HaveOccurred())

				expectedValue := "OFF"
				if enforceClientTLS {
					expectedValue = "ON"
				}

				Expect(DbVariableValue(db, "require_secure_transport")).To(Equal(expectedValue))
			})
		})

		Describe("local-infile", func() {
			var localInfile bool
			BeforeEach(func() {
				var err error
				localInfile, err =
					config.Properties.FindBool("local_infile")
				Expect(err).ToNot(HaveOccurred())
			})

			Context("when local-infile is enabled", func() {
				BeforeEach(func() {
					if !localInfile {
						Skip("local-infile is not enabled")
					}
				})

				It("sets the local_infile system variable to ON", func() {
					localInfile, err := DbVariableValue(db, "local_infile")
					Expect(err).ToNot(HaveOccurred())
					Expect(localInfile).To(Equal("ON"))
				})
			})

			Context("when local-infile is not enabled", func() {
				BeforeEach(func() {
					if localInfile {
						Skip("local-infile is enabled")
					}
				})

				It("sets the local_infile system variable to OFF", func() {
					localInfile, err := DbVariableValue(db, "local_infile")
					Expect(err).ToNot(HaveOccurred())
					Expect(localInfile).To(Equal("OFF"))
				})
			})
		})
	})
})
