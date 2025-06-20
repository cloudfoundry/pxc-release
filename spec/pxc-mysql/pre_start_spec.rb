require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'galera-agent-config template' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '../..')) }
  let(:job) { release.job('pxc-mysql') }
  let(:template) { job.template('bin/pre-start') }
  let(:spec) { {} }
  let(:rendered_template) { template.render(spec) }

  context 'when mysql_version is explicitly set to 8.0' do
    before { spec["mysql_version"] = "8.0" }
    it 'includes a apply_pxc57_crash_recovery function' do
      expect(rendered_template).to include("apply_pxc57_crash_recovery")
    end
  end

  context 'when mysql_version is set to 5.7' do
    before { spec["mysql_version"] = "5.7" }
    it 'does not includes a apply_pxc57_crash_recovery function' do
      expect(rendered_template).to_not include("apply_pxc57_crash_recovery")
    end
  end

  describe 'disable_persistent_storage_safety_check' do
    it 'leaves the safety check on by default' do
      expect(rendered_template).to match(/^check_mysql_disk_persistence/)
    end

    it 'removes the safety check when disabled' do
      spec['disable_persistent_storage_safety_check'] = true
      expect(rendered_template).to_not match(/^check_mysql_disk_persistence/)
    end
  end
end
