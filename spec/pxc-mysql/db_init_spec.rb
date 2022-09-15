require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'


describe 'db_init template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('pxc-mysql') }
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
          "basic-user" => {
            "role" => "minimal",
            "password" => "secret-basic-user-db-pw",
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
    let(:links) { [] }
    let(:rendered_template) { template.render(spec, consumes: links) }


    it 'generates a valid db_init file' do
      expect(rendered_template).to eq File.read(File.join(dir, "db_init_all_features"))
    end

    context 'when a galera-agent link is present' do
      let(:links) {
        [
          Bosh::Template::Test::Link.new(
            name: 'galera-agent',
            properties: {"db_password" => "galera-agent-db-creds"},
            )
        ]
      }
      it 'adds a galera-agent seeded_users entry automatically' do
        expect(rendered_template).to match(/CREATE USER IF NOT EXISTS 'galera-agent'@'localhost'/)
      end
    end

    context 'when a galera-agent link is NOT present' do
      let(:links) { [] }

      it 'adds a galera-agent seeded_users entry automatically' do
        expect(rendered_template).to_not match(/CREATE USER IF NOT EXISTS 'galera-agent'@'localhost'/)
      end
    end

    context 'when a cluster-health-logger link is present' do
      let(:links) {
        [
          Bosh::Template::Test::Link.new(
            name: 'cluster-health-logger',
            properties: {"db_password" => "cluster-health-logger-db-creds"},
            )
        ]
      }
      it 'adds a cluster-health-logger seeded_users entry automatically' do
        expect(rendered_template).to match(/CREATE USER IF NOT EXISTS 'cluster-health-logger'@'localhost'/)
      end
    end

    context 'when a cluster-health-logger link is NOT present' do
      let(:links) { [] }

      it 'adds a cluster-health-logger seeded_users entry automatically' do
        expect(rendered_template).to_not match(/CREATE USER IF NOT EXISTS 'cluster-health-logger'@'localhost'/)
      end
    end

    context 'when both galera-agent and cluster-health-logger links are present' do
      let(:links) {
        [
          Bosh::Template::Test::Link.new(
            name: 'galera-agent',
            properties: {"db_password" => "galera-agent-db-creds"},
            ),
          Bosh::Template::Test::Link.new(
            name: 'cluster-health-logger',
            properties: {"db_password" => "cluster-health-logger-db-creds"},
            )
        ]
      }
      it 'adds a galera-agent seeded_users entry automatically' do
        expect(rendered_template).to match(/CREATE USER IF NOT EXISTS 'galera-agent'@'localhost'/)
      end

      it 'adds a cluster-health-logger seeded_users entry automatically' do
        expect(rendered_template).to match(/CREATE USER IF NOT EXISTS 'cluster-health-logger'@'localhost'/)
      end
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
