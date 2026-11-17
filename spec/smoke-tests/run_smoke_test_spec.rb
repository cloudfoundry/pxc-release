require 'rspec'
require 'bosh/template/test'

describe 'smoke-tests job templates' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('smoke-tests') }

  let(:mysql_link) {
    Bosh::Template::Test::Link.new(
      name: 'mysql',
      address: 'mysql-link-address',
      instances: [
        Bosh::Template::Test::LinkInstance.new(address: 'mysql0-address'),
        Bosh::Template::Test::LinkInstance.new(address: 'mysql1-address'),
      ],
    )
  }

  let(:properties) { { 'db_password' => 'some-password' } }

  describe 'bin/run-smoke-test' do
    let(:template) { job.template('bin/run-smoke-test') }
    let(:rendered) { template.render(properties, consumes: [mysql_link]) }

    it 'does not configure certificate verification by default' do
      expect(rendered).to include('export MYSQL_USERNAME="smoke-tests-user"')
      expect(rendered).not_to include('MYSQL_CA_PATH')
      expect(rendered).not_to include('MYSQL_TLS_SERVER_NAME')
    end

    context 'when tls.ca is set' do
      let(:properties) { super().merge('tls' => { 'ca' => 'some-ca' }) }

      it 'exports the CA path but no server name override' do
        expect(rendered).to include('export MYSQL_CA_PATH="/var/vcap/jobs/smoke-tests/config/ca.pem"')
        expect(rendered).not_to include('MYSQL_TLS_SERVER_NAME')
      end
    end

    context 'when tls.server_name is set' do
      let(:properties) { super().merge('tls' => { 'ca' => 'some-ca', 'server_name' => 'some-server-name' }) }

      it 'exports the server name override' do
        expect(rendered).to include('export MYSQL_TLS_SERVER_NAME="some-server-name"')
      end
    end
  end

  describe 'config/ca.pem' do
    let(:template) { job.template('config/ca.pem') }

    it 'is empty by default' do
      expect(template.render(properties).strip).to be_empty
    end

    it 'contains the configured CA' do
      pem = "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"
      rendered = template.render(properties.merge('tls' => { 'ca' => pem }))
      expect(rendered.strip).to eq(pem)
    end
  end
end
