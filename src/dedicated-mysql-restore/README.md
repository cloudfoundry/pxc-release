# dedicated-mysql-restore

## Compile:

```
go install dedicated-mysql-restore
```


## How to run:

```
$ ./restore --help

Usage:
  restore [OPTIONS]

Application Options:
      --encryption-key= Key used to decrypt backup artifact
      --mysql-username= Username to authenticate to mysql instance
      --mysql-password= Password to authenticate to mysql instance

Help Options:
  -h, --help            Show this help message
```

Examples (done inside mysql VM):

```
## Create a restore directory
$ mkdir /var/vcap/store/restore-artifact/

## Stop all services
$ monit stop all

## Compress and encrypte data directory
$ tar -c -C /var/vcap/store/mysql/data/ . | \
    gpg --batch -c --cipher-algo AES256 --passphrase=SECRET > \
    /var/vcap/store/restore-artifact/mysql-backup.tar.gpg

## Decrypt your encrypted backup
gpg -d --passphrase=secret < /var/vcap/store/restore-artifact/mysql-backup.tar.gpg | tar -tv


## Restore from your decrypted backup
$ ./restore --encryption-key=SECRET --mysql-username=ADMIN --mysql-password=PASSWORD
```
