require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'galera-agent-config template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('pxc-mysql') }
  let(:template) { job.template('config/bpm.yml') }
  let(:spec) { {} }

  it 'set default PATH for MySQL 8.0' do
    tpl_output = template.render(spec)
    hash_from_yaml = YAML.load(tpl_output)
    expect(hash_from_yaml["processes"][0]["env"]["PATH"]).to eq('/usr/bin:/bin:/var/vcap/packages/percona-xtradb-cluster-8.0/bin/')
  end

  it 'set PATH for MySQL 5.7' do
    spec["mysql_version"] = "5.7"
    tpl_output = template.render(spec)
    hash_from_yaml = YAML.load(tpl_output)
    expect(hash_from_yaml["processes"][0]["env"]["PATH"]).to eq('/usr/bin:/bin:/var/vcap/packages/percona-xtradb-cluster-5.7/bin/')
  end
end
