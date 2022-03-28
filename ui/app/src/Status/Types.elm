module Status.Types exposing (ClusterPeer, ClusterStatus, VersionInfo)


type alias StatusResponse =
    { config : String
    , uptime : String
    , versionInfo : VersionInfo
    , clusterStatus : Maybe ClusterStatus
    }


type alias VersionInfo =
    { branch : String
    , buildDate : String
    , buildUser : String
    , goVersion : String
    , revision : String
    , version : String
    }


type alias ClusterStatus =
    { name : String
    , status : String
    , peers : List ClusterPeer
    }


type alias ClusterPeer =
    { name : String
    , address : String
    }
