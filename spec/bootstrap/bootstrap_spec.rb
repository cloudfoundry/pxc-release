require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'bootstrap job' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
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
