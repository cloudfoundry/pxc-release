require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'galera init-config template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
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
