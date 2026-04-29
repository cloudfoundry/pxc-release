require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'galera-agent-config template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('galera-agent') }
  let(:links) {[
      Bosh::Template::Test::Link.new(
        name: 'mysql',
        instances: [Bosh::Template::Test::LinkInstance.new(address: 'mysql-address')],
        properties: { "mysql_version" => "8.0"}
      )
    ]}
  let(:template) { job.template('config/galera-agent-config.yml') }
  let(:spec) { {} }

  before do
    spec["db_password"] = "db_password"
    spec["endpoint_password"] = "endpoint_password"
  end

  it 'renders GaleraInit (not Monit) with defaults for galera-init HTTP and state file' do
    tpl_output = template.render(spec, consumes: links)
    hash_from_yaml = YAML.load(tpl_output)

    expect(hash_from_yaml).to_not have_key('Monit')
    expect(hash_from_yaml).to include('GaleraInit')
    expect(hash_from_yaml['GaleraInit']['GaleraInitStatusServerAddress']).to eq('127.0.0.1:8114')
    expect(hash_from_yaml['GaleraInit']['ServiceName']).to eq('galera-init')
    expect(hash_from_yaml['GaleraInit']['MysqlStateFilePath']).to eq('/var/vcap/store/pxc-mysql/state.txt')
  end

  it 'set default MysqldPath for MySQL 8.0' do
    tpl_output = template.render(spec, consumes: links)
    hash_from_yaml = YAML.load(tpl_output)

    expect(hash_from_yaml).to include("MysqldPath")
    expect(hash_from_yaml["MysqldPath"]).to match('/var/vcap/packages/percona-xtradb-cluster-8.0/bin/mysqld')
  end
end
