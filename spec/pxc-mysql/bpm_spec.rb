require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'galera-agent-config template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('pxc-mysql') }
  let(:template) { job.template('config/bpm.yml') }
  let(:spec) { {} }
  let(:rendered_template) { template.render(spec) }
  let(:parsed_bpm_yaml) { YAML.load(rendered_template) }

  it 'set default PATH for Percona XtraDB Cluster 8.0' do
    expect(parsed_bpm_yaml["processes"][0]["env"]["PATH"]).to eq('/usr/bin:/bin:/var/vcap/packages/percona-xtradb-cluster-8.0/bin')
  end

  context 'when mysql_version is explicitly set to 8.4' do
    before { spec["mysql_version"] = "8.4" }
    it 'set PATH for Percona XtraDB Cluster 8.4' do
      expect(parsed_bpm_yaml["processes"][0]["env"]["PATH"]).to eq('/usr/bin:/bin:/var/vcap/packages/percona-xtradb-cluster-8.4/bin')
    end
  end

  context 'when mysql_version is explicitly set to 8.0' do
    before { spec["mysql_version"] = "8.0" }
    it 'set PATH for Percona XtraDB Cluster 8.0' do
      expect(parsed_bpm_yaml["processes"][0]["env"]["PATH"]).to eq('/usr/bin:/bin:/var/vcap/packages/percona-xtradb-cluster-8.0/bin')
    end
  end

  context 'when jemalloc is enabled' do
    before { spec["engine_config"] = { "jemalloc" => { "enabled" => true } } }

    it 'loads the jemalloc by setting the LD_PRELOAD environment variable' do
      expect(parsed_bpm_yaml["processes"][0]["env"]["LD_PRELOAD"]).to eq('/var/vcap/packages/jemalloc/lib/libjemalloc.so.2')
    end

    context 'when profiling is enabled' do
      before { spec["engine_config"]["jemalloc"]["profiling"] = true }
      it 'enables jemalloc profiling by setting the MALLOC_CONF environment variable' do
        expect(parsed_bpm_yaml["processes"][0]["env"]["MALLOC_CONF"]).to eq('prof:true')
      end
    end
  end
end
