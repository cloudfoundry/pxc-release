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
      expect(rendered_template).to_not match(/^check_mysql_disk_persistence$/)
    end
  end

  supported_plugins = [
    { plugin: 'mysql_native_password' },
    { plugin: 'caching_sha2_password' }
  ]
  supported_plugins.each do |tc|
    context 'valid user_auth_plugin configuration' do
      before { spec["user_auth_plugin"] = tc[:plugin] }
      it "supports the #{tc[:plugin]} plugin" do
        expect { template.render(spec) }.not_to raise_error
      end
    end
  end

  unsupported_plugins = [
     { plugin: 'authentication_ldap_sasl' },
     { plugin: 'mysql_clear_password' },
     { plugin: 'something_that_doesnt_exist' }
   ]
  unsupported_plugins.each do |tc|
    context 'unsupported user_auth_plugin configuration' do
      before { spec["user_auth_plugin"] = tc[:plugin] }
      it "raises an error for #{tc[:plugin]}" do
        expect { template.render(spec) }.to raise_error(RuntimeError, "Unsupported value '#{tc[:plugin]}' for 'user_auth_plugin' property. Choose from 'mysql_native_password' or 'caching_sha2_password'")
      end
    end
  end
end

