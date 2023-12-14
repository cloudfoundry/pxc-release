require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'prom_scraper_config.yml.erb configuration template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('proxy') }
  let(:template) { job.template('config/prom_scraper_config.yml') }
  let(:spec) { {}}
  let(:rendered_template) { template.render(spec) }
  let(:parsed_config) {
    YAML.load(rendered_template)
  }

  it 'renders an empty file' do
    expect(parsed_config).to be_nil
  end

  context 'when proxy prometheus support is enabled' do
    before do
      spec['metrics'] = {
        'enabled' => true,
      }
    end
    context 'when relying on default values' do
      it 'configures the prom scraper to scrape the proxy prometheus endpoint' do
        expect(parsed_config).to include("port" => 9999)
        expect(parsed_config).to include("source_id" => "pxc-proxy")
        expect(parsed_config).to include("scheme" => "http")
        expect(parsed_config).to include("labels" => {})
      end

      it 'configures the prom scraper to emit events with source_id and instance_id tags' do
        expect(parsed_config['source_id']).to eq('pxc-proxy')
        expect(parsed_config['instance_id']).to_not be_empty
      end
    end

    context 'when spec values are provided' do
      before {
        spec["metrics"]["port"] = 1234
        spec["metrics"]["source_id"] = "service-instance-guid-pxc-proxy"
        spec["metrics"]["scheme"] = "http"
        spec["metrics"]["server_name"] = "my-proxy-tls"
        spec["metrics"]["labels"] = { "origin": "p-mysql", "my-tag": "my-value"}
        spec["api_tls"] = {
          "enabled" => true
        }
      }
      it 'configures the prom scraper to scrape the proxy prometheus endpoint' do
        expect(parsed_config).to include("port" => 1234)
        expect(parsed_config).to include("scheme" => "https")
        expect(parsed_config).to include("server_name" => "my-proxy-tls")
      end

      it 'configures the prom scraper to emit events with source_id and instance_id tags' do
        expect(parsed_config).to include("source_id" => "service-instance-guid-pxc-proxy")
        expect(parsed_config['instance_id']).to_not be_empty
      end

      it 'configures additional metric labels based on the spec property' do
        expect(parsed_config).to include("labels" => {"origin" => "p-mysql", "my-tag" => "my-value"})
      end

      context 'when metric labels are misconfigured' do
        before(:each) do
          spec["metrics"]["labels"] = 42
        end

        it 'raises an error' do
            expect { rendered_template }.to raise_error(RuntimeError, "metrics.labels must be a hash but got 42")
        end
      end
    end
  end
end