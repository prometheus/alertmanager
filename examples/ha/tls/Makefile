# Based on https://github.com/wolfeidau/golang-massl/

.PHONY: start
start:
	goreman start

.PHONY: gen-certs
gen-certs: certs/ca.pem certs/node1.pem certs/node1-key.pem certs/node2.pem certs/node2-key.pem

certs/ca.pem certs/ca-key.pem: certs/ca-csr.json
	cd certs; cfssl gencert -initca ca-csr.json | cfssljson -bare ca

certs/node1.pem certs/node1-key.pem: certs/ca-config.json certs/ca.pem certs/ca-key.pem certs/node1-csr.json
	cd certs; cfssl gencert  \
    -ca=ca.pem \
    -ca-key=ca-key.pem \
    -config=ca-config.json \
    -hostname=localhost,127.0.0.1 \
    -profile=massl node1-csr.json | cfssljson -bare node1

certs/node2.pem certs/node2-key.pem: certs/ca-config.json certs/ca.pem certs/ca-key.pem certs/node2-csr.json
	cd certs; cfssl gencert  \
    -ca=ca.pem \
    -ca-key=ca-key.pem \
    -config=ca-config.json \
    -hostname=localhost,127.0.0.1 \
    -profile=massl node2-csr.json | cfssljson -bare node2
