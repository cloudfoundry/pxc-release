require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'proxy.yml.erb configuration template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('proxy') }
  let(:links) { [
    Bosh::Template::Test::Link.new(
      name: 'proxy',
      instances: [
        Bosh::Template::Test::LinkInstance.new(index: 0, address: 'proxy0-address'),
        Bosh::Template::Test::LinkInstance.new(index: 1, address: 'proxy1-address'),
      ],
    ),

    Bosh::Template::Test::Link.new(
      name: 'mysql',
      instances: [
        Bosh::Template::Test::LinkInstance.new(id: "mysql0-uuid", address: 'mysql0-address'),
        Bosh::Template::Test::LinkInstance.new(id: "mysql1-uuid", address: 'mysql1-address'),
        Bosh::Template::Test::LinkInstance.new(id: "mysql2-uuid", address: 'mysql2-address'),
      ],
      properties: {
        "port" => 6033,
      }
    ),
    Bosh::Template::Test::Link.new(
      name: 'galera-agent',
      properties: {
        "port" => "9201",
        "endpoint_tls" => {
          "enabled" => true,
          "ca" => "PEM Cert",
          "server_name" => "server name"
        }
      }
    )
  ] }
  let(:template) { job.template('config/proxy.yml') }
  let(:spec) {
    {
      "api_password" => "random-switchboard-password",
      "healthcheck_timeout_millis" => 12345,
      "api_uri" => "proxy.some-platform.domain",
      "api_tls" => { "enabled" => true, "certificate" => "proxy-api-cert", "private_key" => "proxy-api-private-key" }
    }
  }
  let(:parsed_config) {
    YAML.load(template.render(spec, consumes: links))
  }

  it 'renders a valid proxy.yml' do
    expected_config = {
      "API" => {
        "AggregatorPort" => 8082,
        "ForceHttps" => true,
        "Password" => "random-switchboard-password",
        "Port" => 8080,
        "ProxyURIs" => %w[0-proxy.some-platform.domain 1-proxy.some-platform.domain],
        "Username" => "proxy",
        "TLS" => {
          "Enabled" => true,
          "Certificate" => "proxy-api-cert",
          "PrivateKey" => "proxy-api-private-key",
        },
      },

      "Proxy" => {
        "Port" => 3306,
        "HealthcheckTimeoutMillis" => 12345,
        "Backends" => [
          { "Host" => "mysql0-address", "Name" => "backend-0", "Port" => 6033, "StatusEndpoint" => "api/v1/status", "StatusPort" => "9201" },
          { "Host" => "mysql1-address", "Name" => "backend-1", "Port" => 6033, "StatusEndpoint" => "api/v1/status", "StatusPort" => "9201" },
          { "Host" => "mysql2-address", "Name" => "backend-2", "Port" => 6033, "StatusEndpoint" => "api/v1/status", "StatusPort" => "9201" },
        ],
      },
      "HealthPort" => 1936,
      "StaticDir" => '/var/vcap/packages/proxy/static',
      "PidFile" => "/var/vcap/sys/run/proxy/proxy.pid",
      "GaleraAgentTLS" => {
        "Enabled" => true,
        "CA" => "PEM Cert",
        "ServerName" => "server name",
      }
    }
    expect(parsed_config).to eq(expected_config)
  end

  context 'when galera-agent disables tls' do
    before(:each) do
      links.select { |l| l.name == "galera-agent" }[0].properties["endpoint_tls"] = {
        "enabled" => false
      }
    end

    it 'does not configure GaleraAgentTLS' do
      expect(parsed_config).to_not include("GaleraAgentTLS")
    end
  end

  context 'the proxy api tls is disabled' do
    before(:each) do
      spec["api_tls"]["enabled"] = false
    end

    it 'does not configure GaleraAgentTLS' do
      expect(parsed_config["API"]).to_not have_key("TLS")
    end
  end

  context 'when inactive_mysql_port is configured' do
    before(:each) { spec["inactive_mysql_port"] = 3307 }

    it 'configures the InactiveMySQLPort property' do
      expect(parsed_config["Proxy"]).to include("InactiveMysqlPort" => 3307)
    end
  end
end
