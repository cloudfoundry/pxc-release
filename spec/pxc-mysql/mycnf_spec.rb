require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'
require 'iniparse'

describe 'my.cnf template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('pxc-mysql') }
  let(:links) { [
    Bosh::Template::Test::Link.new(
      name: 'mysql',
      instances: [Bosh::Template::Test::LinkInstance.new(address: 'mysql-address')],
      properties: {}
    ),
    Bosh::Template::Test::Link.new(
      name: 'galera-agent',
      properties: {
        "endpoint_tls" => {
          "enabled" => true,
          "ca" => "PEM Cert",
          "server_name" => "server name"
        }
      }
    )
  ] }
  let(:template) { job.template('config/my.cnf') }
  let(:spec) { {} }
  let(:rendered_template) { template.render(spec, consumes: links) }
  let(:parsed_mycnf) {
    # Comment out my.cnf !include* directives to avoid parsing failures
    rendered_template_cleaned = rendered_template.gsub(/^(!include.*)/, '#\1')
    # Convert common bare option names to "$option = true" for testing
    rendered_template_cleaned = rendered_template_cleaned.gsub(/((?:skip|disable)[_-][a-z_-]+)/, '\1 = true')

    # Convert rendered my.cnf to a parsed hash, preserving duplicate keys
    # i.e. "plugin-load-add=foo.so\nplugin-load-add=bar.so" => { "plugin-load-add" => ["foo.so","bar.so"]}
    IniParse.parse(rendered_template_cleaned).
      reduce({}) do |h, section|
      h.update(
        section.key => section.reduce({}) do |opts, o|
          opts.update(o.key => opts.key?(o.key) ? [opts[o.key]].flatten << o.value : o.value)
        end
      )
    end
  }

  it 'sets the authentication-policy' do
    expect(rendered_template).to match(/authentication-policy\s*=\s*mysql_native_password/)
  end

  context 'when no explicit collation is set' do
    it 'uses the default mysql collation and does not configure a collation in the my.cnf' do
      expect(rendered_template).to_not match(/collation[-_]server/)
    end
  end

  context 'when an explicit collation is set' do
    let(:spec) { { "engine_config" => { "character_set_server" => "armscii8", "collation_server" => "armscii8_general_ci" } } }
    it 'configures that collation' do
      expect(rendered_template).to match(/collation_server\s+=\s+armscii8_general_ci/)
    end

    # pxc-5.7 does not understand all the collations in PXC 8.0
    # since we use pxc-5.7 for crash recovery and would like to generally read _other_ options pxc-8.0 specific changes
    # are in the [mysqld-8.0] config section
    it 'supports pxc-5.7 still reading this config by putting charset/collation options in the [mysqld] section' do
      expect(rendered_template).to match(/\[mysqld\]\ncharacter_set_server\s+=\s+armscii8\ncollation_server\s+=\s+armscii8_general_ci/m)
    end
  end

  context 'binlog_expire_logs_seconds' do
    it 'renders the correct binlog_expire_logs_seconds from a day value' do
      expect(rendered_template).to match("binlog_expire_logs_seconds.*=.*604800")
    end
  end

  context 'global properties are as expected ' do
    it 'sets max-connections' do
      expect(rendered_template).to match(/max_connections\s*=\s*5000/)
    end
  end

  context 'tls.required is enabled ' do
    before do
      spec["tls"] = { "required" => true }
    end

    it 'enables require-secure-transport' do
      expect(rendered_template).to include("require-secure-transport        = ON")
    end
  end
  context 'tls.required is disabled' do
    before do
      spec["tls"] = { "required" => false }
    end

    it 'does not enable require-secure-transport' do
      expect(rendered_template).not_to include("require-secure-transport")
    end
  end
  context 'tls.required is not set' do
    before do
      spec.delete("tls")
    end

    it 'does not set require-secure-transport' do
      expect(rendered_template).not_to include("require-secure-transport")
    end
  end

  context 'mysql 8.0' do
    it 'suppresses warnings about deprecated features to mitigate excessive logging' do
      expect(parsed_mycnf).to include("mysqld-8.0" => hash_including("log-error-suppression-list" => "ER_SERVER_WARN_DEPRECATED"))
    end
  end

  context 'when galera is not enabled' do
    let(:spec) { {
      "engine_config" => {
        "galera" => {
          "enabled" => false
        }
      }
    } }

    context 'read_write_permissions' do
      it 'configures the super-read-only option if read_write_permissions specified as "super_read_only"' do
        spec["engine_config"]["read_write_permissions"] = "super_read_only"
        expect(rendered_template).to include("super-read-only = ON")
      end

      it 'configures the read-only option if read_write_permissions specified as "read_only"' do
        spec["engine_config"]["read_write_permissions"] = "read_only"
        expect(rendered_template).to include("read-only = ON")
        expect(rendered_template).not_to include("super-read-only = ON")
      end

      it 'does not set read-only options if read_write_permissions are not specified' do
        expect(rendered_template).not_to include("read-only")
        expect(rendered_template).not_to include("super-read-only")
      end
    end

    context 'when gtid_mode has not been explicitly configured' do
      it 'turns gtid_mode and enforce_gtid_consistency on by default' do
        expect(rendered_template).to include("gtid_mode = ON")
        expect(rendered_template).to include("enforce_gtid_consistency = ON")
      end
    end

    context 'when gtid_mode has been explicitly enabled' do
      it 'turns gtid_mode and enforce_gtid_consistency on by user request' do
        spec["engine_config"] = { "binlog" => { "enable_gtid_mode" => true } }

        expect(rendered_template).to include("gtid_mode = ON"), "expected gtid_mode to be set in the my.cnf, but it was not"
        expect(rendered_template).to include("enforce_gtid_consistency = ON"), "expected enforce_gtid_consistency to be set in the my.cnf, but it was not"
      end
    end

    context 'when gtid_mode is explicitly disabled' do
      it 'does not configure the gtid_mode and enforce_gtid_consistency options' do
        spec["engine_config"] = { "binlog" => { "enable_gtid_mode" => false } }

        expect(rendered_template).not_to include("gtid_mode"), "expected gtid_mode not to be rendered in the my.cnf"
        expect(rendered_template).not_to include("enforce_gtid_consistency"), "expected enforce_gtid_consistency not to be rendered in the my.cnf"
      end
    end
  end

  context 'when galera is enabled' do
    let(:spec) { {
      "admin_username" => "foo",
      "admin_password" => "bar",
      "engine_config" => {
        "galera" => {
          "enabled" => true
        }
      }
    } }

    it 'sets wsrep_sst_auth for 5.7' do
      expect(rendered_template).to match(/\[mysqld-5\.7\]\nwsrep_sst_auth/m)
    end

    context 'when audit logs are disabled (default)' do
      it 'has no audit log format' do
        expect(rendered_template).not_to include("audit_log_format")
      end
    end

    context 'when audit logs are enabled' do
      before do
        spec["engine_config"]["audit_logs"] = { "enabled" => true }
      end

      it 'exists in [mysqld_plugin] group' do
        expect(rendered_template).to match(/\[mysqld_plugin\]\s+/)
      end

      it 'has audit log format' do
        expect(rendered_template).to match(/audit_log_format\s+= JSON/)
      end

      it 'defaults audit_log_policy to ALL' do
        expect(rendered_template).to match(/audit_log_policy\s+= ALL/)
      end

      it 'excludes system accounts from the audit logs' do
        expect(rendered_template).to match(/audit_log_exclude_accounts\s*=.*'galera-agent'@'localhost'.*/)
        expect(rendered_template).to match(/audit_log_exclude_accounts\s*=.*'cluster-health-logger'@'localhost'.*/)
      end
    end

    context 'when audit logs are enabled with a non default value' do
      before do
        spec["engine_config"]["audit_logs"] = { "enabled" => true }
        spec["engine_config"]["audit_logs"]["audit_log_policy"] = "some-policy"
      end

      it 'exists in [mysqld_plugin] group' do
        expect(rendered_template).to match(/\[mysqld_plugin\]\s+/)
      end

      it 'has audit log format' do
        expect(rendered_template).to match(/audit_log_format\s+= JSON/)
      end

      it 'sets the audit_log_policy based on the property' do
        expect(rendered_template).to match(/audit_log_policy\s+= some-policy/)
      end
    end

    context 'read_write_permissions' do
      it 'configures the super-read-only option if read_write_permissions specified as "super_read_only"' do
        spec["engine_config"]["read_write_permissions"] = "super_read_only"
        expect(rendered_template).to include("super-read-only = ON")
      end

      it 'configures the read-only option if read_write_permissions specified as "read_only"' do
        spec["engine_config"]["read_write_permissions"] = "read_only"
        expect(rendered_template).to include("read-only = ON")
        expect(rendered_template).not_to include("super-read-only = ON")
      end

      it 'does not set read-only options if read_write_permissions are not specified' do
        expect(rendered_template).not_to include("read-only")
        expect(rendered_template).not_to include("super-read-only")
      end
    end

    context 'when gtid_mode has not been explicitly configured' do
      it 'does NOT turn gtid_mode and enforce_gtid_consistency on by default' do
        expect(rendered_template).not_to include("gtid_mode"), "expected gtid_mode not to be rendered in the my.cnf"
        expect(rendered_template).not_to include("enforce_gtid_consistency"), "expected enforce_gtid_consistency not to be rendered in the my.cnf"
      end
    end

    context 'when gtid_mode has been explicitly enabled' do
      it 'turns gtid_mode and enforce_gtid_consistency on by user request' do
        spec["engine_config"] = { "binlog" => { "enable_gtid_mode" => true } }

        expect(rendered_template).to include("gtid_mode = ON"), "expected gtid_mode to be set in the my.cnf, but it was not"
        expect(rendered_template).to include("enforce_gtid_consistency = ON"), "expected enforce_gtid_consistency to be set in the my.cnf, but it was not"
      end
    end

    context 'when gtid_mode is explicitly disabled' do
      it 'does not configure the gtid_mode and enforce_gtid_consistency options' do
        spec["engine_config"] = { "binlog" => { "enable_gtid_mode" => false } }

        expect(rendered_template).not_to include("gtid_mode"), "expected gtid_mode not to be rendered in the my.cnf"
        expect(rendered_template).not_to include("enforce_gtid_consistency"), "expected enforce_gtid_consistency not to be rendered in the my.cnf"
      end
    end

    it 'defaults to no wsrep_applier_threads for mysql 8.0' do
      expect(rendered_template).not_to include("wsrep_applier_threads")
    end

    it 'defaults to no wsrep_slave_threads for mysql 5.7' do
      expect(rendered_template).not_to include("wsrep_slave_threads")
    end

    context 'engine_config.galera.wsrep_applier_threads is explicitly configured' do
      let(:spec) { {
        "admin_username" => "foo",
        "admin_password" => "bar",
        "engine_config" => {
          "galera" => {
            "enabled" => true,
            "wsrep_applier_threads" => 32
          }
        }
      } }

      it 'configures wsrep_applier_threads to that value' do
        expect(rendered_template).to match(/wsrep_applier_threads\s+= 32/)
      end

      it 'configures wsrep_slave_threads to that value' do
        expect(rendered_template).to match(/wsrep_slave_threads\s+= 32/)
      end
    end
  end

  it 'enables binary logs by default' do
    expect(parsed_mycnf).to include("mysqld" => hash_including("log_bin" => "mysql-bin"))
  end

  context 'when engine_config.binlog.enabled is true' do
    it 'enables binary logs' do
      spec["engine_config"] = { "binlog" => { "enabled" => true } }
      expect(parsed_mycnf).to include("mysqld" => hash_including("log_bin" => "mysql-bin"))
    end
  end

  context 'when engine_config.binlog.enabled is false' do
    it 'disables binary logs' do
      spec["engine_config"] = { "binlog" => { "enabled" => false } }
      expect(parsed_mycnf).to include("mysqld" => hash_including("skip-log-bin" => true))
    end
  end

  # https://docs.percona.com/percona-server/8.0/jemalloc-profiling.html
  context 'when jemalloc profiling is enabled' do
    before { spec["engine_config"] = { "jemalloc" => { "enabled" => true, "profiling" => true } } }

    it 'enables the Percona jemalloc-profiling option for mysql-8.0' do
      expect(parsed_mycnf).to include("mysqld-8.0" => hash_including("jemalloc-profiling" => "ON"))
    end

    # The jemalloc-profiling feature is only supported in Percona v8.0.25+
    it 'does not attempt to enable jemalloc-profiling for mysql-5.7' do
      expect(parsed_mycnf).to_not include("mysqld" => hash_including("jemalloc-profiling" => "ON"))
      expect(parsed_mycnf).to_not include("mysqld-5.7" => hash_including("jemalloc-profiling" => "ON"))
    end
  end

  context 'when no explicit innodb_flush_method is set' do
    it 'defaults innodb_flush_method to fsync' do
      expect(rendered_template).to match("innodb_flush_method\s+= fsync")
    end
  end

  context 'when an explicit innodb_flush_method is set' do
    let(:spec) { { "engine_config" => { "innodb_flush_method" => "O_DIRECT"} } }
    it 'configures that innodb_flush_method' do
      expect(rendered_template).to match("innodb_flush_method\s+= O_DIRECT")
    end
  end

  context 'when an invalid innodb_flush_method is set' do
    let(:spec) { { "engine_config" => { "innodb_flush_method" => "INVALID"} } }
    it 'throws an exception' do
      expect { template.render(spec, consumes: links) }.to raise_error(RuntimeError, "Only innodb_flush_method=O_DIRECT or fsync or unset are supported!")
    end
  end

  context 'when config provides additional mysql "raw entry" properties' do

    let(:spec) { {
      "engine_config" => {
        "additional_raw_entries" => {
          "mysql" => {
            "early-plugin-load" => "arbitrary-plugin-name",
            "arbitrary-mysql-param" => "arbitrary-mysql-param-value"
          },
          "client" => {
            "additional-client-property" => "additional-client-value"
          },
          "mysql-8.0" => {
            "additional-mysql-8.0-property" => "additional-mysql-8.0-value"
          },
          "mysql-5.7" => {
            "additional-mysql-5.7-property" => "additional-mysql-5.7-value"
          },
          "mysqld" => {
            "additional-mysqld-property" => "additional-mysqld-value"
          },
          "mysqld_plugin" => {
            "additional-mysqld_plugin-property" => "additional-mysqld_plugin-value"
          },
          "sst" => {
            "additional-sst-property" => "additional-sst-value"
          },
          "mysqldump" => {
            "additional-mysqldump-property" => "additional-mysqldump-value"
          },
          "new_section" => {
            "new_section-property" => "new_section-value"
          }
        }
      }
    } }

    it 'adds the additional entries to my.cnf as expected' do
      expect(parsed_mycnf).to include("mysql" => hash_including("early-plugin-load" => "arbitrary-plugin-name"))
      expect(parsed_mycnf).to include("mysql" => hash_including("arbitrary-mysql-param" => "arbitrary-mysql-param-value"))
    end
    it 'does not effect existing my.cnf contents' do
      expect(parsed_mycnf).to include("mysql" => hash_including("max_allowed_packet" => "256M"))
    end

    it 'adds additional provided client properties' do
      expect(parsed_mycnf).to include("client" => hash_including("additional-client-property" => "additional-client-value"))
    end
    it 'adds additional provided mysql-8.0 properties' do
      expect(parsed_mycnf).to include("mysql-8.0" => hash_including("additional-mysql-8.0-property" => "additional-mysql-8.0-value"))
    end
    it 'adds additional provided mysql-5.7 properties' do
      expect(parsed_mycnf).to include("mysql-5.7" => hash_including("additional-mysql-5.7-property" => "additional-mysql-5.7-value"))
    end
    it 'adds additional provided mysqld properties' do
      expect(parsed_mycnf).to include("mysqld" => hash_including("additional-mysqld-property" => "additional-mysqld-value"))
    end
    it 'adds additional provided mysqld_plugin properties' do
      expect(parsed_mycnf).to include("mysqld_plugin" => hash_including("additional-mysqld_plugin-property" => "additional-mysqld_plugin-value"))
    end
    it 'adds additional provided sst properties' do
      expect(parsed_mycnf).to include("sst" => hash_including("additional-sst-property" => "additional-sst-value"))
    end
    it 'adds additional provided mysqldump properties' do
      expect(parsed_mycnf).to include("mysqldump" => hash_including("additional-mysqldump-property" => "additional-mysqldump-value"))
    end
    it 'adds provided properties in new config sections outside the supported set' do
      expect(parsed_mycnf).to include("new_section")
      expect(parsed_mycnf).to include("new_section" => hash_including("new_section-property" => "new_section-value"))
    end

  end
end
