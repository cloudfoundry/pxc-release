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

  context 'when mysql_version is explicitly set to 8.0' do
    before { spec["mysql_version"] = "8.0" }
    it 'set PATH for Percona XtraDB Cluster 5.7' do
      expect(parsed_bpm_yaml["processes"][0]["env"]["PATH"]).to eq('/usr/bin:/bin:/var/vcap/packages/percona-xtradb-cluster-8.0/bin')
    end
  end

  context 'when mysql_version is set to 5.7' do
    before { spec["mysql_version"] = "5.7" }
    it 'set PATH for Percona XtraDB Cluster 5.7' do
      expect(parsed_bpm_yaml["processes"][0]["env"]["PATH"]).to eq('/usr/bin:/bin:/var/vcap/packages/percona-xtradb-cluster-5.7/bin:/var/vcap/packages/percona-xtrabackup-2.4/bin')
    end
  end
end
