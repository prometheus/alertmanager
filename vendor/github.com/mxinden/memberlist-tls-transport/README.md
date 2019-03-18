# Memberlist TLS Transport

**Status: Prove of concept**

This is a fork of [Memberlist](https://github.com/hashicorp/memberlist)'s
[`NetTransport`](https://github.com/hashicorp/memberlist/blob/master/net_transport.go)
implementing the
[`Transport`](https://godoc.org/github.com/hashicorp/memberlist#Transport)
interface.

Instead of using TCP and UDP like the default transport layer of _Memberlist_,
`TLSTransport` uses only TCP for both reliable as well as best-effort
communication. On top of that TCP layer, `TLSTransport` uses mutual TLS to
secure all communication.
