module Views.ReceiverBar.Types exposing (Model, Msg(..), initReceiverBar)

import Alerts.Types exposing (Receiver)
import Utils.Types exposing (ApiData(..))


type Msg
    = ReceiversFetched (ApiData (List Receiver))
    | UpdateReceiver String
    | EditReceivers
    | FilterByReceiver String
    | Select (Maybe Receiver)
    | ResultsHovered Bool
    | BlurReceiverField
    | Noop


type alias Model =
    { receivers : List Receiver
    , matches : List Receiver
    , fieldText : String
    , selectedReceiver : Maybe Receiver
    , showReceivers : Bool
    , resultsHovered : Bool
    }


initReceiverBar : Model
initReceiverBar =
    { receivers = []
    , matches = []
    , fieldText = ""
    , selectedReceiver = Nothing
    , showReceivers = False
    , resultsHovered = False
    }
