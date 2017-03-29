module Alerts.Types exposing (Alert, AlertGroup, Block, RouteOpts)

import Utils.Types exposing (Labels)
import Time exposing (Time)


type alias Alert =
    { annotations : Labels
    , labels : Labels
    , inhibited : Bool
    , silenceId : Maybe String
    , silenced : Bool
    , startsAt : Time
    , generatorUrl : String
    }


type alias AlertGroup =
    { blocks : List Block
    , labels : Labels
    }


type alias Block =
    { alerts : List Alert
    , routeOpts : RouteOpts
    }


type alias RouteOpts =
    { receiver : String }
