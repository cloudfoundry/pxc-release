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
      expect(template.render(spec, consumes: links)).to match(/CREATE USER IF NOT EXISTS 'galera-agent'@'localhost'/)
    end
  end

  context 'when a galera-agent link is NOT present' do
    let(:links) { [] }

    it 'adds a galera-agent seeded_users entry automatically' do
      expect(template.render(spec, consumes: links)).to_not match(/CREATE USER IF NOT EXISTS 'galera-agent'@'localhost'/)
    end
  end

end
