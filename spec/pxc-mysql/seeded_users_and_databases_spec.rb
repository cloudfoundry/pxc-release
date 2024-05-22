require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'db_init template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('pxc-mysql') }
  let(:template) { job.template('config/seeded_users_and_databases.sql') }
  let(:spec) { { "admin_password" => "secret-admin-pw" } }
  let(:dir) { File.join(File.dirname(__FILE__), "golden") }

  context 'specifying users via various spec properties' do
    let(:spec) {
      {
        "admin_password" => "secret-admin-pw",
        "mysql_backup_password" => "secret-backup-pw",
        "seeded_databases" => [
          {
            "name" => "metrics_db",
            "username" => "mysql-metrics",
            "password" => "ignored-password-overridden-by-seeded-users-entry"
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
            "role" => "mysql-metrics",
            "password" => "secret-mysql-metrics-db-pw",
            "host" => "any",
            "schema" => "metrics_db",
            "max_user_connections" => 3,
          },
          "multi-schema-admin-user" => {
            "role" => "multi-schema-admin",
            "password" => "secret-multi-schema-admin-db-pw",
            "host" => "any",
            "schema" => "multi_schemas_%",
          },
        }
      }
    }
    let(:links) { [] }
    let(:rendered_template) { template.render(spec, consumes: links) }

    it 'generates a valid sql file' do
      expect(rendered_template).to eq File.read(File.join(dir, "seeded_users_and_databases_all_features"))
    end

    context 'when seeded_users specifies an empty username' do
      before(:each) do
        spec["seeded_users"][""] = { "host" => "any", "password" => "foo", "role" => "minimal" }
      end

      it 'fails' do
        expect { template.render(spec) }.to raise_error(RuntimeError, "seeded_users property specifies an empty username")
      end
    end

    context 'when seeded_users specifies an empty password' do
      before(:each) do
        spec["seeded_users"]["invalid-user"] = { "host" => "any", "password" => "", "role" => "minimal" }
      end

      it 'fails' do
        expect { template.render(spec) }.to raise_error(RuntimeError, "seeded_users property specifies an empty allowed 'password' for username invalid-user")
      end
    end

    context 'when seeded_users fails to specify a password' do
      before(:each) do
        spec["seeded_users"]["invalid-user"] = { "host" => "any", "role" => "minimal" }
      end

      it 'fails' do
        expect { template.render(spec) }.to raise_error(RuntimeError, "seeded_users property specifies an empty allowed 'password' for username invalid-user")
      end
    end

    context 'when seeded_users fails to specify a role' do
      before(:each) do
        spec["seeded_users"]["invalid-user"] = { "host" => "any", "password" => "secret" }
      end

      it 'fails' do
        expect { template.render(spec) }.to raise_error(RuntimeError, "seeded_users property specifies an empty allowed 'role' for username invalid-user")
      end
    end

    context 'when seeded_users specifies an unsupported host' do
      before(:each) do
        spec["seeded_users"]["invalid-user"] = { "host" => "unsupported-value", "password" => "secret", "role" => "minimal" }
      end

      it 'fails' do
        expect { template.render(spec) }.to raise_error(RuntimeError, "invalid host 'unsupported-value' specified for username invalid-user")
      end
    end

    context 'when seeded_users specifies an invalid-role' do
      before(:each) do
        spec["seeded_users"]["invalid-user"] = { "host" => "any", "password" => "secret", "role" => "unsupported-role" }
      end

      it 'fails' do
        expect { template.render(spec) }.to raise_error(RuntimeError, "Unsupported role 'unsupported-role' for user 'invalid-user'")
      end
    end

    context 'when seeded_users specifies a schema-admin role without a schema' do
      before(:each) do
        spec["seeded_users"]["invalid-schema-admin-user"] = { "host" => "any", "password" => "secret", "role" => "schema-admin" }
      end

      it 'fails' do
        expect { template.render(spec) }.to raise_error(RuntimeError, "user 'invalid-schema-admin-user' with schema-admin role specified with an empty schema")
      end
    end

    context 'when seeded_users specifies a multi-schema-admin role without a schema' do
      before(:each) do
        spec["seeded_users"]["invalid-schema-admin-user"] = { "host" => "any", "password" => "secret", "role" => "multi-schema-admin" }
      end

      it 'fails' do
        expect { template.render(spec) }.to raise_error(RuntimeError, "user 'invalid-schema-admin-user' with multi-schema-admin role specified with an empty schema")
      end
    end
  end

end