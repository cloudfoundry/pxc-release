require 'rspec'
require 'yaml'
require 'bosh/template/test'

describe 'galera-agent bpm template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('galera-agent') }
  let(:template) { job.template('config/bpm.yml') }
  let(:spec) { { 'logging.format.timestamp' => 'rfc3339' } }
  let(:rendered_template) { template.render(spec) }
  let(:parsed) { YAML.load(rendered_template) }

  it 'does not use unsafe or privileged BPM configuration' do
    expect(rendered_template).to_not include('unsafe')
    expect(rendered_template).to_not include('privileged')
    expect(parsed).to_not have_key('unsafe')
  end

  it 'runs the packaged binary directly (no wrapper script) with config and timestamp args' do
    process = parsed['processes'].first
    expect(process['name']).to eq('galera-agent')
    expect(process['executable']).to eq('/var/vcap/packages/galera-agent/bin/galera-agent')
    expect(process['args']).to eq([
      '--configPath=/var/vcap/jobs/galera-agent/config/galera-agent-config.yml',
      '--timeFormat=rfc3339',
    ])
    paths = process['additional_volumes'].map { |v| v['path'] }
    expect(paths).to include('/var/vcap/sys/run/pxc-mysql', '/var/vcap/jobs/pxc-mysql', '/var/vcap/store')
  end
end
