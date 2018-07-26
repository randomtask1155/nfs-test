

## deployment

update the `FILENAME` in the manifest.yml

```
---
applications:
  - name: nfs-test
    memory: 128M
    instances: 1
    buildpack: https://github.com/kr/heroku-buildpack-go.git
    command: nfstest
    env:
      FILENAME: "FILENAME_IN_NFS_SHARE"
```

Then push it 

```
cf push
```

## run workload test

the below example will run read the file on the nfs share every 10 miliseconds

```
nfstest:> curl -k https://nfs-test.domain.io/api/run?interval=10
success
```

## get metrics

```
nfstest:> curl -k https://nfs-test.domain.io/api/metrics
{"avg-read-ms":0.8645495714285713,"max-read-ms":3.538586,"min-read-ms":0.52729,"rate-second":91}
```

## stop test

```
nfstest:> curl -k https://nfs-test.domain.io/api/stop
success
```

