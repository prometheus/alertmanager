a1: ./alertmanager --log.level=debug --storage.path=$TMPDIR/a1 --web.listen-address=:9093  --cluster.listen-address=127.0.0.1:8001 --config.file=examples/ha/alertmanager.yml
a2: ./alertmanager --log.level=debug --storage.path=$TMPDIR/a2 --web.listen-address=:9094  --cluster.listen-address=127.0.0.1:8002 --cluster.peer=127.0.0.1:8001 --config.file=examples/ha/alertmanager.yml
a3: ./alertmanager --log.level=debug --storage.path=$TMPDIR/a3 --web.listen-address=:9095  --cluster.listen-address=127.0.0.1:8003 --cluster.peer=127.0.0.1:8001 --config.file=examples/ha/alertmanager.yml
wh: go run ./examples/webhook/echo.go

