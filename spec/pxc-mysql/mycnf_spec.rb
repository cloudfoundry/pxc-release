require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

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
  let(:rendered_template) {template.render(spec, consumes: links) }

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
      expect(rendered_template).to match(/\[mysqld]\ncharacter_set_server\s+=\s+armscii8\ncollation_server\s+=\s+armscii8_general_ci/m)
    end
  end

  context 'binlog_expire_logs_seconds' do
    it 'renders the correct binlog_expire_logs_seconds from a day value' do
      expect(rendered_template).to match("binlog_expire_logs_seconds.*=.*604800")
    end
  end

  context 'global properties are as expected ' do
    it 'sets max-connections' do
      expect(rendered_template).to match(/max_connections[\s]*=[\s]*5000/)
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

  context 'when galera is not enabled' do
    let(:spec) { {
      "engine_config" => {
        "galera" => {
          "enabled" => false
        }
      }
    } }

    it 'set super-read-only if read_write_permissions specified' do
      spec["engine_config"]["read_write_permissions"] = "super_read_only"
      expect(rendered_template).to include("super-read-only = ON")
    end

    it 'set read-only if read_write_permissions specified' do
      spec["engine_config"]["read_write_permissions"] = "read_only"
      expect(rendered_template).to include("read-only = ON")
      expect(rendered_template).not_to include("super-read-only = ON")
    end

    it 'do nothing if read_write_permissions not specified' do
      expect(rendered_template).not_to include("read-only = ON")
      expect(rendered_template).not_to include("super-read-only = ON")
    end

    it 'turns gtid_mode and enforce_gtid_consistency on' do
      expect(rendered_template).to include("gtid_mode = ON")
      expect(rendered_template).to include("enforce_gtid_consistency = ON")
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
      expect(rendered_template).to match(/\[mysqld-5.7]\nwsrep_sst_auth/m)
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

    it 'do nothing if read_write_permissions specified' do
      spec["engine_config"]["read_write_permissions"] = "super_read_only"
      expect(rendered_template).not_to include("read-only = ON")
      expect(rendered_template).not_to include("super-read-only = ON")
    end

    it 'do nothing if read_write_permissions specified' do
      spec["engine_config"]["read_write_permissions"] = "read_only"
      expect(rendered_template).not_to include("read-only = ON")
      expect(rendered_template).not_to include("super-read-only = ON")
    end

    it 'do nothing if read_write_permissions not specified' do
      expect(rendered_template).not_to include("read-only = ON")
      expect(rendered_template).not_to include("super-read-only = ON")
    end

    it 'keeps gtid_mode and enforce_gtid_consistency off' do
      expect(rendered_template).not_to include("gtid_mode = ON")
      expect(rendered_template).not_to include("enforce_gtid_consistency = ON")
    end

    it 'defaults Galera applier threads to 1' do
      expect(rendered_template).to match(/wsrep_applier_threads\s+= 1/)
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
    end

  end
end
