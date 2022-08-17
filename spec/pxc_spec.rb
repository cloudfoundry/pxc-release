require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'bootstrap job' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '..')) }
  let(:job) { release.job('bootstrap') }
  let(:links) {[
    Bosh::Template::Test::Link.new(
      name: 'galera-agent',
      instances: [Bosh::Template::Test::LinkInstance.new(address: 'IP1')],
      properties: {
        "port" => 42,
        "endpoint_username" => "username",
        "endpoint_password" => "hunter2",
        "endpoint_tls" => {
            "enabled" => true,
            "ca" => "PEM Cert",
            "server_name" => "server name"
        }
      }
    )
  ]}

  describe 'bootstrap config template' do
    let(:template) { job.template('config/config.yml') }
    let(:spec) { {} }
    context 'tls.required is enabled ' do
      it 'enables require-secure-transport' do
        bootstrap_output = template.render(spec, consumes: links)
        expect(bootstrap_output).to include("https://IP1:42")
      end
    end
    context 'tls.required is not enabled ' do
      before do
        links.first.properties["endpoint_tls"]["enabled"] = false
      end
      it 'enables require-secure-transport' do
        bootstrap_output = template.render(spec, consumes: links)
        expect(bootstrap_output).to include("http://IP1:42")
      end
    end
  end
end

describe 'pxc mysql job' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '..')) }
  let(:job) { release.job('pxc-mysql') }
  let(:links) {[
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
  ]}

  describe 'galera init-config template' do
    let(:template) { job.template('config/galera-init-config.yml') }
    let(:spec) { {} }

    before do
      spec["admin_password"] = "test"
    end

    it 'renders a valid galera-init-config.yml' do
      tpl_output = template.render(spec, consumes: links)
      hash_from_yaml = YAML.load(tpl_output)

      expect(hash_from_yaml).to include("Db")

      expect(hash_from_yaml["Db"]).to include("SkipBinlog"=>true)

      expect(hash_from_yaml).to include("Manager")

      expect(hash_from_yaml["Manager"]).to include("ClusterIps" => ["mysql-address"])

      expect(hash_from_yaml).to include("BackendTLS" => {"CA"=>"PEM Cert", "Enabled"=>true, "ServerName"=>"server name"})
    end
  end

  describe 'my.cnf template' do
    let(:template) { job.template('config/my.cnf') }
    let(:spec) { {} }

    it 'sets the authentication-policy' do
    	tpl_output = template.render(spec, consumes: links)
    	expect(tpl_output).to include("authentication-policy=mysql_native_password")
   	end

    context 'binlog_expire_logs_seconds' do
        it 'renders the correct binlog_expire_logs_seconds from a day value' do
            tpl_output = template.render(spec, consumes: links)
            expect(tpl_output).to match("binlog_expire_logs_seconds.*=.*604800")
        end
    end
    context 'tls.required is enabled ' do
      before do
          spec["tls"] = { "required" => true }
      end

      it 'enables require-secure-transport' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to include("require-secure-transport=ON")
      end
    end
    context 'tls.required is disabled' do
      before do
          spec["tls"] = { "required" => false }
      end

      it 'does not enable require-secure-transport' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("require-secure-transport")
      end
    end
    context 'tls.required is not set' do
      before do
          spec.delete("tls")
      end

      it 'does not set require-secure-transport' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("require-secure-transport")
      end
    end

    context 'when galera is not enabled' do
      let(:spec) {{
        "engine_config" => {
          "galera" => {
            "enabled" => false
          }
        }
      }}

      it 'set super-read-only if read_write_permissions specified' do
        spec["engine_config"]["read_write_permissions"] = "super_read_only"
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to include("super-read-only = ON")
      end

      it 'set read-only if read_write_permissions specified' do
        spec["engine_config"]["read_write_permissions"] = "read_only"
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end

      it 'do nothing if read_write_permissions not specified' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end

      it 'turns gtid_mode and enforce_gtid_consistency on' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to include("gtid_mode = ON")
        expect(tpl_output).to include("enforce_gtid_consistency = ON")
      end

      it 'uses the sync binlog setting of 1 to sync to disk immediately' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to match(/sync_binlog[\s]*=[\s]*1/)
      end
    end

    context 'when galera is enabled' do
      let(:spec) {{
        "admin_username" => "foo",
        "admin_password" => "bar",
        "engine_config" => {
          "galera" => {
            "enabled" => true
          }
        }
      }}

      it 'does not set the wsrep_sst_auth' do
       	tpl_output = template.render(spec, consumes: links)
       	expect(tpl_output).not_to include("wsrep_sst_auth")
      end

      context 'when audit logs are disabled (default)' do
        it 'has no audit log format' do
            tpl_output = template.render(spec, consumes: links)
            expect(tpl_output).not_to include("audit_log_format")
        end
      end

      context 'when audit logs are enabled' do
        before do
            spec["engine_config"]["audit_logs"] = { "enabled" => true }
        end

        it 'exists in [mysqld_plugin] group' do
			tpl_output = template.render(spec, consumes: links)
			expect(tpl_output).to match(/\[mysqld_plugin\]\s+/)
		end

        it 'has audit log format' do
            tpl_output = template.render(spec, consumes: links)
            expect(tpl_output).to match(/audit_log_format\s+= JSON/)
        end

        it 'defaults audit_log_policy to ALL' do
            tpl_output = template.render(spec, consumes: links)
            expect(tpl_output).to match(/audit_log_policy\s+= ALL/)
        end

        it 'excludes system accounts from the audit logs' do
          tpl_output = template.render(spec, consumes: links)
          expect(tpl_output).to match(/audit_log_exclude_accounts\s*=.*'galera-agent'@'localhost'.*/)
          expect(tpl_output).to match(/audit_log_exclude_accounts\s*=.*'cluster-health-logger'@'localhost'.*/)
        end
      end

      context 'when audit logs are enabled with a non default value' do
        before do
            spec["engine_config"]["audit_logs"] = { "enabled" => true }
            spec["engine_config"]["audit_logs"]["audit_log_policy"] = "some-policy"
        end

        it 'exists in [mysqld_plugin] group' do
           tpl_output = template.render(spec, consumes: links)
           expect(tpl_output).to match(/\[mysqld_plugin\]\s+/)
		end

        it 'has audit log format' do
            tpl_output = template.render(spec, consumes: links)
            expect(tpl_output).to match(/audit_log_format\s+= JSON/)
        end

        it 'sets the audit_log_policy based on the property' do
            tpl_output = template.render(spec, consumes: links)
            expect(tpl_output).to match(/audit_log_policy\s+= some-policy/)
        end
      end

      it 'do nothing if read_write_permissions specified' do
        spec["engine_config"]["read_write_permissions"] = "super_read_only"
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end

      it 'do nothing if read_write_permissions specified' do
        spec["engine_config"]["read_write_permissions"] = "read_only"
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end

      it 'do nothing if read_write_permissions not specified' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end

      it 'keeps gtid_mode and enforce_gtid_consistency off' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("gtid_mode = ON")
        expect(tpl_output).not_to include("enforce_gtid_consistency = ON")
      end

      it 'defaults Galera applier threads to 1' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to match(/wsrep_applier_threads\s+= 1/)
      end

      it 'allows mysql to default to the sync binlog setting of 0 which does not sync to disk immediately' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("sync_binlog")
      end

      context 'engine_config.galera.wsrep_applier_threads is explicitly configured' do
            let(:spec) {{
              "admin_username" => "foo",
              "admin_password" => "bar",
              "engine_config" => {
                "galera" => {
                  "enabled" => true,
                  "wsrep_applier_threads" => 32
                }
              }
            }}

          it 'configures wsrep_applier_threads to that value' do
            tpl_output = template.render(spec, consumes: links)
            expect(tpl_output).to match(/wsrep_applier_threads\s+= 32/)
          end
      end

    end
  end

  describe 'db_init template' do
    let(:template) { job.template('config/db_init') }
    let(:spec) { {} }
    let(:dir) { File.join(File.dirname(__FILE__), "golden")}

    it 'requires an admin_password' do
      expect { template.render(spec) }.to raise_error(Bosh::Template::UnknownProperty, "Can't find property '[\"admin_password\"]'")
    end


    context 'when admin_password is configured correctly' do
      let(:spec) {
        {
          "admin_password" => "secret-admin-pw",
          "roadmin_enabled" => true,
          "roadmin_password" => "secret-roadmin-pw",
          "mysql_backup_password" => "secret-backup-pw",
          "seeded_databases" => [
            {
              "name" => "metrics_db",
              "username" => "mysql-metrics",
              "password" => "ignored-password-overriden-by-seeded-users-entry"
            },
            {
              "name" => "cloud_controller",
              "username" => "cloud_controller",
              "password" => "secret-ccdb-pw",
            }
          ],
          "seeded_users" => {
            "galera-agent" => {
              "role" => "minimal",
              "password" => "secret-galera-agent-db-pw",
              "host" => "localhost",
            },
            "special-admin-user" => {
              "role" => "admin",
              "password" => "secret-seeded-admin-pw",
              "host" => "any",
            },
            "mysql-metrics" => {
              "role" => "schema-admin",
              "password" => "secret-mysql-metrics-db-pw",
              "host" => "any",
              "schema" => "metrics_db",
            },
          }
        }
      }
      let(:rendered_template) { template.render(spec) }


      it 'generates a valid db_init file' do
        expect(rendered_template).to eq File.read(File.join(dir, "db_init_all_features"))
      end


      context 'when roadmin_password is not specified' do
        before(:each) do
          spec.delete("roadmin_password")
        end

        it 'fails fast because roadmin_password is required when read-only admin is enabled' do
          expect { template.render(spec) }.to raise_error(Bosh::Template::UnknownProperty, "Can't find property '[\"roadmin_password\"]'")
        end
      end

      context 'when the read-only admin feature is not enabled' do
        before(:each) do
          spec["roadmin_enabled"] = false
        end

        it 'does not initialize the roadmin user' do
          expect(rendered_template).to eq File.read(File.join(dir, "db_init_no_roadmin"))
        end
      end

      context 'when the mysql-backup user is not enabled' do
        before(:each) do
          spec.delete('mysql_backup_password')
        end

        it 'does not initialize the mysql-backup user' do
          expect(rendered_template).to eq File.read(File.join(dir, "db_init_no_mysqlbackup"))
        end
      end

      context 'when seeded_users specifies an empty username' do
        before(:each) do
          spec["seeded_users"][""] = { "host" => "any", "password" => "foo", "role" => "minimal"}
        end

        it 'fails' do
          expect { template.render(spec) }.to raise_error(RuntimeError, "seeded_users property specifies an empty username")
        end
      end

      context 'when seeded_users specifies an empty password' do
        before(:each) do
          spec["seeded_users"]["invalid-user"] = { "host" => "any", "password" => "", "role" => "minimal"}
        end

        it 'fails' do
          expect { template.render(spec) }.to raise_error(RuntimeError, "seeded_users property specifies an empty allowed 'password' for username invalid-user")
        end
      end

      context 'when seeded_users fails to specify a password' do
        before(:each) do
          spec["seeded_users"]["invalid-user"] = { "host" => "any", "role" => "minimal"}
        end

        it 'fails' do
          expect { template.render(spec) }.to raise_error(RuntimeError, "seeded_users property specifies an empty allowed 'password' for username invalid-user")
        end
      end

      context 'when seeded_users fails to specify a role' do
        before(:each) do
          spec["seeded_users"]["invalid-user"] = { "host" => "any", "password" => "secret"}
        end

        it 'fails' do
          expect { template.render(spec) }.to raise_error(RuntimeError, "seeded_users property specifies an empty allowed 'role' for username invalid-user")
        end
      end

      context 'when seeded_users specifies an unsupported host' do
        before(:each) do
          spec["seeded_users"]["invalid-user"] = { "host" => "unsupported-value", "password" => "secret", "role" => "minimal"}
        end

        it 'fails' do
          expect { template.render(spec) }.to raise_error(RuntimeError, "invalid host 'unsupported-value' specified for username invalid-user")
        end
      end

      context 'when seeded_users specifies an invalid-role' do
        before(:each) do
          spec["seeded_users"]["invalid-user"] = { "host" => "any", "password" => "secret", "role" => "unsupported-role"}
        end

        it 'fails' do
          expect { template.render(spec) }.to raise_error(RuntimeError, "Unsupported role 'unsupported-role' for user 'invalid-user'")
        end
      end

      context 'when seeded_users specifies a schema-admin role without a schema' do
        before(:each) do
          spec["seeded_users"]["invalid-schema-admin-user"] = { "host" => "any", "password" => "secret", "role" => "schema-admin"}
        end

        it 'fails' do
          expect { template.render(spec) }.to raise_error(RuntimeError, "user 'invalid-schema-admin-user' with schema-admin role specified with an empty schema")
        end
      end
    end

  end
end


