---
TITLE: MariaDB Control Script
---

This repository contains a go process to manage the start process for MariaDB in [cf-mysql-release] (https://github.com/cloudfoundry/cf-mysql-release).

### Run unit tests

```
./bin/test-unit
```

### Run integration tests

With default DB configuration:
```
./bin/test-integration
```

Default Options:
```
Host: localhost
Port: 3306
User: root
Password: ''
```

Override defaults:
```
CONFIG="{Password: 'password'}" ./bin/test-integration
```
