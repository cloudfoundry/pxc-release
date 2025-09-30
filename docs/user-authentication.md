# User authentication

## Background
Historically, PXC release has set the authentication plugin to 'mysql_native_password', the default authentication for MySQL 5.7.
As support was added for MySQL 8.0, the decision was made to continue using that plugin rather than the more secure 'caching_sha2_password' which is the default in 8.0.
The intention was to make the migration as easy on users as possible.

As the team begins to prepare for MySQL 8.4, it was decided to start enabling users to migrate away from 'mysql_native_password', which is deprecated in 8.0, disabled by default in 8.4, and removed completely in 9.0

To enable this transition, new job properties were added to allow consumers of PXC release to begin configuring deployment manifests now to help with the transition. 'mysql_native_password' is still the default, but it can be configured now so that consumers can validate their applications and MySQL clients support the newer 'caching_sha2_password' plugin.

## Default values
If not explicitly set, the default value for 'engine_config.user_authentication_policy' is 'mysql_native_password'

If not explicitly set for an individual user, the default value for 'seeded_users.CUSTOM_USER.auth_plugin' is the value of 'engine_config.user_authentication_policy'

## Supported plugins
Currently, PXC release supports only 'mysql_native_password' and 'caching_sha2_password'. Any other values will be rejected as part of the deployment process.

## Configuration
At the system level:
```
engine_config:
  user_authentication_policy: 'mysql_native_password'
```
The value set for this property will be used to configure all users, unless explicitly overridden for the individual user.

At the individual user level:
```
seeded_users:
  admin:
    password: "((mysql_root_password))"
    host: loopback
    role: admin
  app-user-with-custom-plugin:
    password: "((app_user_password))"
    host: any
    role: schema-admin
    schema: "app-db"
    auth_plugin: caching_sha2_password
```
When setting both the top-level 'user_authentication_policy' and user-specific 'auth_plugin', the user-specific value takes higher priority.
This allows consumers to test their applications one user at a time to validate support.