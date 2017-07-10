module Views.ReceiverBar.Types exposing (Model, Msg(..), initReceiverBar)

import Utils.Types exposing (ApiData(Initial))


type Msg
    = ReceiversFetched (ApiData (List String))
    | ToggleReceivers Bool
    | SelectReceiver (Maybe String)


type alias Model =
    { receivers : List String
    , showRecievers : Bool
    }


initReceiverBar : Model
initReceiverBar =
    { receivers = []
    , showRecievers = False
    }
