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

      it 'turns gtid_mode and enforce_gtid_consistency on' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to include("gtid_mode = ON")
        expect(tpl_output).to include("enforce_gtid_consistency = ON")
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

      it 'keeps gtid_mode and enforce_gtid_consistency off' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).not_to include("gtid_mode = ON")
        expect(tpl_output).not_to include("enforce_gtid_consistency = ON")
      end

      it 'defaults Galera applier threads to 1' do
        tpl_output = template.render(spec, consumes: links)
        expect(tpl_output).to match(/wsrep_slave_threads\s+= 1/)
      end

      context 'engine_config.galera.wsrep_applier_threads is explicitly configured' do
            let(:spec) {{
              "admin_username" => "foo",
              "admin_password" => "bar",
              "engine_config" => {
                "galera" => {
                  "enabled" => true,
                  "wsrep_applier_threads" => 32
                }
              }
            }}

          it 'configures wsrep_slave_threads to that value' do
            tpl_output = template.render(spec, consumes: links)
            expect(tpl_output).to match(/wsrep_slave_threads\s+= 32/)
          end
      end

    end
  end
end


