# cluster.reconnect
* alertmanager->cluster.go -> Peer.reconnect
  * memeberlist -> memberlist.go -> MemberList.Join -> MemberList.pushPullNode.
* How alertmanager send nf log data to remote node?
  * where is nf log data?
* How alertmanager use nf log?
  * In DedupStage.Exec, it will query nf log.
* What is nf log?
  * Where intialization of nflog
    * main.go: notificationLog, err := nflog.New(notificationLogOpts...)