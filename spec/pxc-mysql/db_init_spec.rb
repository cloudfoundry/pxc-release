require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'db_init template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('pxc-mysql') }
  let(:template) { job.template('config/db_init') }
  let(:spec) { { "admin_password" => "secret-admin-pw" } }
  let(:dir) { File.join(File.dirname(__FILE__), "golden") }

  context 'when an admin_password was not provided' do
    let(:spec) {}
    it 'fails' do
      expect { template.render(spec) }.to raise_error(Bosh::Template::UnknownProperty, "Can't find property '[\"admin_password\"]'")
    end
  end

  context 'when creating a database via a user entry' do
    before(:each) { spec["seeded_databases"] = [{ "name" => "metrics_db", "username" => "metrics-user", "password" => "metrics_password" }] }
    context 'when the the default collation is used' do
      it 'creates a database by specifying only the character set' do
        create_db_statement = <<~SQL
          SELECT COUNT(*) INTO @_schema_exists FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = 'metrics_db';
          SET @_sql = IF(@_schema_exists, 'DO 1', 'CREATE SCHEMA `metrics_db` CHARACTER SET ''utf8mb4''');
          PREPARE stmt FROM @_sql;
          EXECUTE stmt;
          DROP PREPARE stmt;
        SQL

        expect(template.render(spec)).to include(create_db_statement)
      end
    end

    context 'when the spec is configured explicitly with the collation name "use_default"' do
      it 'creates a database by specifying only the character set' do
        create_db_statement = <<~SQL
          SELECT COUNT(*) INTO @_schema_exists FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = 'metrics_db';
          SET @_sql = IF(@_schema_exists, 'DO 1', 'CREATE SCHEMA `metrics_db` CHARACTER SET ''utf8mb4''');
          PREPARE stmt FROM @_sql;
          EXECUTE stmt;
          DROP PREPARE stmt;
        SQL

        expect(template.render(spec)).to include(create_db_statement)
      end
    end

    context 'when a unique charset / collation is specified' do
      before(:each) do
        spec["engine_config"] = { "character_set_server" => "latin7", "collation_server" => "latin7_estonian_cs" }
      end

      it 'creates a database by specifying only the character set' do
        create_db_statement = <<~SQL
          SELECT COUNT(*) INTO @_schema_exists FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = 'metrics_db';
          SET @_sql = IF(@_schema_exists, 'DO 1', 'CREATE SCHEMA `metrics_db` CHARACTER SET ''latin7'' COLLATE ''latin7_estonian_cs''');
          PREPARE stmt FROM @_sql;
          EXECUTE stmt;
          DROP PREPARE stmt;
        SQL

        expect(template.render(spec)).to include(create_db_statement)
      end
    end
  end

  context 'when roadmin_enabled is specified' do
    before(:each) { spec["roadmin_enabled"] = true }
    context 'when roadmin_password was not provided' do
      it 'fails' do
        expect { template.render(spec) }.to raise_error(Bosh::Template::UnknownProperty, "Can't find property '[\"roadmin_password\"]'")
      end
    end

    context 'when roadmin_password was specified' do
      before(:each) { spec["roadmin_password"] = "secret-roadmin-pw" }

      def roadmin_user_for_host(host)
        <<~SQL
          CREATE USER IF NOT EXISTS 'roadmin'@'#{host}' IDENTIFIED WITH mysql_native_password BY 'secret-roadmin-pw';
          ALTER USER 'roadmin'@'#{host}' IDENTIFIED WITH mysql_native_password BY 'secret-roadmin-pw';
          GRANT SELECT, PROCESS, REPLICATION CLIENT ON *.* TO 'roadmin'@'#{host}';
        SQL
      end

      it 'renders SQL to create an roadmin user for all localhost access types' do
        expect(template.render(spec)).to include(roadmin_user_for_host("localhost"))
        expect(template.render(spec)).to include(roadmin_user_for_host("127.0.0.1"))
        expect(template.render(spec)).to include(roadmin_user_for_host("::1"))
      end
    end
  end

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

    it 'generates a valid db_init file' do
      expect(rendered_template).to eq File.read(File.join(dir, "db_init_all_features"))
    end

    context 'when mysql_version is set to "5.7"' do
      before { spec["mysql_version"] = "5.7" }
      it 'still generates generates a valid db_init file' do
        expect(rendered_template).to eq File.read(File.join(dir, "db_init_all_features_mysql57"))
      end
    end

    context 'when a galera-agent link is present' do
      let(:links) {
        [
          Bosh::Template::Test::Link.new(
            name: 'galera-agent',
            properties: { "db_password" => "galera-agent-db-creds" },
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
            properties: { "db_password" => "cluster-health-logger-db-creds" },
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
            properties: { "db_password" => "galera-agent-db-creds" },
          ),
          Bosh::Template::Test::Link.new(
            name: 'cluster-health-logger',
            properties: { "db_password" => "cluster-health-logger-db-creds" },
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
