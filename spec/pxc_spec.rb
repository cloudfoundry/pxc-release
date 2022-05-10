require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

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

      array = []
      hash1 = {"username" => "username", "user_config" => {}}
      array.push(hash1)
      spec["seeded_databases"] = [
		{
			"name" => "test",
			"username" => "test-user",
			"password" => "test-password"
		},
		{
			"name" => "test1",
			"username" => "test-user1",
			"password" => "test-password1"
		}
      ]
      spec["seeded_users"] = [
      		[
      			"user1",
      			{"password" => "test-password1","host" => "host1","role" => "role1"}
      		],
			[
				"user2",
				{"password" => "test-password2","host" => "host2","role" => "role2"}
			]
      ]
    end

    it 'fails' do
      tpl_output = template.render(spec, consumes: links)
      File.open("./galera-init-config.yml", 'w') { |file| file.write(tpl_output) }
      expect(tpl_output).to include("Db")
      expect(tpl_output).to include("SkipBinlog: true")

      expect(tpl_output).to include("SeededUsers")
      expect(tpl_output).to include("Host: host1")

      expect(tpl_output).to include("PreseededDatabases")
      expect(tpl_output).to include("DBName: test")

      expect(tpl_output).to include("Upgrader")

      expect(tpl_output).to include("Manager")

      expect(tpl_output).to include("ClusterIps")
      expect(tpl_output).to include("- mysql-address")

      expect(tpl_output).to include("BackEndTLS")
      expect(tpl_output).to include("Enabled: true")
    end
  end

  describe 'my.cnf template' do
    let(:template) { job.template('config/my.cnf') }
    let(:spec) { {} }

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
        expect(tpl_output).to match(/wsrep_slave_threads\s+= 1/)
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

          it 'configures wsrep_slave_threads to that value' do
            tpl_output = template.render(spec, consumes: links)
            expect(tpl_output).to match(/wsrep_slave_threads\s+= 32/)
          end
      end

    end
  end
end


