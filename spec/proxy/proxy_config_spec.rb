require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'proxy.yml.erb configuration template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('proxy') }

  let(:proxy_link) {
    Bosh::Template::Test::Link.new(
      name: 'proxy',
      instances: [
        Bosh::Template::Test::LinkInstance.new(index: 0, address: 'proxy0-address'),
        Bosh::Template::Test::LinkInstance.new(index: 1, address: 'proxy1-address'),
      ],
    )
  }

  let(:mysql_link) {
    Bosh::Template::Test::Link.new(
      name: 'mysql',
      instances: [
        Bosh::Template::Test::LinkInstance.new(name: "mysql", id: "mysql0-uuid", address: 'mysql0-address'),
        Bosh::Template::Test::LinkInstance.new(name: "mysql", id: "mysql1-uuid", address: 'mysql1-address'),
        Bosh::Template::Test::LinkInstance.new(name: "mysql", id: "mysql2-uuid", address: 'mysql2-address'),
      ],
      properties: {
        "port" => 6033,
      }
    )
  }

  let(:galera_agent_link) {
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
  }

  let(:links) { [proxy_link, mysql_link, galera_agent_link] }

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
          { "Host" => "mysql0-address", "Name" => "mysql/mysql0-uuid", "Port" => 6033, "StatusEndpoint" => "api/v1/status", "StatusPort" => "9201" },
          { "Host" => "mysql1-address", "Name" => "mysql/mysql1-uuid", "Port" => 6033, "StatusEndpoint" => "api/v1/status", "StatusPort" => "9201" },
          { "Host" => "mysql2-address", "Name" => "mysql/mysql2-uuid", "Port" => 6033, "StatusEndpoint" => "api/v1/status", "StatusPort" => "9201" },
        ],
      },
      "HealthPort" => 1936,
      "StaticDir" => '/var/vcap/packages/proxy/static',
      "GaleraAgentTLS" => {
        "Enabled" => true,
        "CA" => "PEM Cert",
        "ServerName" => "server name",
      }
    }
    expect(parsed_config).to eq(expected_config)
  end

  context 'when the mysql link is for database instance group' do
    let(:mysql_link) {
      Bosh::Template::Test::Link.new(
        name: 'mysql',
        instances: [
          Bosh::Template::Test::LinkInstance.new(name: "database", id: "mysql0-uuid", address: 'mysql0-address'),
          Bosh::Template::Test::LinkInstance.new(name: "database", id: "mysql1-uuid", address: 'mysql1-address'),
          Bosh::Template::Test::LinkInstance.new(name: "database", id: "mysql2-uuid", address: 'mysql2-address'),
        ],
        properties: {
          "port" => 6033,
        }
      )
    }

    it 'names the backends based on the instance group' do
      expect(parsed_config["Proxy"]["Backends"]).to match([
        include("Name" => "database/mysql0-uuid"),
        include("Name" => "database/mysql1-uuid"),
        include("Name" => "database/mysql2-uuid"),
      ])
    end
  end

  context 'when galera-agent disables tls' do
    let(:galera_agent_link) {
      Bosh::Template::Test::Link.new(
        name: 'galera-agent',
        properties: {
          "port" => "9200",
          "endpoint_tls" => {
            "enabled" => false,
          }
        }
      )
    }

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
