module Views.ReceiverBar.Types exposing (Model, Msg(..), Receiver, apiReceiverToReceiver, initReceiverBar)

import Browser.Navigation exposing (Key)
import Data.Receiver
import Regex
import Utils.Types exposing (ApiData(..))


type Msg
    = ReceiversFetched (ApiData (List Data.Receiver.Receiver))
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
    , key : Key
    }


type alias Receiver =
    { name : String
    , regex : String
    }


escapeRegExp : String -> String
escapeRegExp text =
    let
        reg =
            Regex.fromString "[-[\\]{}()*+?.,\\\\^$|#\\s]" |> Maybe.withDefault Regex.never
    in
    Regex.replace reg (.match >> (++) "\\") text


apiReceiverToReceiver : Data.Receiver.Receiver -> Receiver
apiReceiverToReceiver r =
    Receiver r.name (escapeRegExp r.name)


initReceiverBar : Key -> Model
initReceiverBar key =
    { receivers = []
    , matches = []
    , fieldText = ""
    , selectedReceiver = Nothing
    , showReceivers = False
    , resultsHovered = False
    , key = key
    }
