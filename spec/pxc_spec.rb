require 'rspec'
require 'json'
require 'yaml'
require 'bosh/template/test'

describe 'pxc mysql job' do
  let(:release) { Bosh::Template::Test::ReleaseDir.new(File.join(File.dirname(__FILE__), '..')) }
  let(:job) { release.job('pxc-mysql') }
  let(:links) {[
    Bosh::Template::Test::Link.new(
      name: 'mysql',
      instances: [Bosh::Template::Test::LinkInstance.new(address: 'mysql-address')],
      properties: {}
    )
  ]}

  describe 'my.cnf template' do
    let(:template) { job.template('config/my.cnf') }
    context 'when galera is not enabled' do
      let(:spec) {{
        "engine_config" => {
          "galera" => {
            "enabled" => false
          }
        }
      }}

      it 'set super-read-only if read_write_permissions specified' do
        spec["engine_config"]["read_write_permissions"] = "super_read_only"
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to include("super-read-only = ON")
      end

      it 'set read-only if read_write_permissions specified' do
        spec["engine_config"]["read_write_permissions"] = "read_only"
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end

      it 'do nothing if read_write_permissions not specified' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end
    end

    context 'when galera is enabled' do
      let(:spec) {{
        "admin_username" => "foo",
        "admin_password" => "bar",
        "engine_config" => {
          "galera" => {
            "enabled" => true
          }
        }
      }}

      it 'do nothing if read_write_permissions specified' do
        spec["engine_config"]["read_write_permissions"] = "super_read_only"
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end

      it 'do nothing if read_write_permissions specified' do
        spec["engine_config"]["read_write_permissions"] = "read_only"
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end

      it 'do nothing if read_write_permissions not specified' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("read-only = ON")
        expect(tpl_output).not_to include("super-read-only = ON")
      end
    end
  end
end


