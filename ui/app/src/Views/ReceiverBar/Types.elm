module Views.ReceiverBar.Types exposing (Model, Msg(..), Receiver, apiReceiverToReceiver, initReceiverBar, updateDebouncer)

import Browser.Navigation exposing (Key)
import Data.Receiver
import Debouncer.Messages as Debouncer exposing (Debouncer, fromSeconds, settleWhenQuietFor, toDebouncer)
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
    | FilterReceiverList
    | DebounceReceiverFilter (Debouncer.Msg Msg)


type alias Model =
    { receivers : List Receiver
    , matches : List Receiver
    , receiverDebouncer : Debouncer Msg
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


updateDebouncer : Debouncer.UpdateConfig Msg Model
updateDebouncer =
    { mapMsg = DebounceReceiverFilter
    , getDebouncer = .receiverDebouncer
    , setDebouncer = \debouncer model -> { model | receiverDebouncer = debouncer }
    }


escapeRegExp : String -> String
escapeRegExp text =
    let
        reg =
            Regex.fromString "/[-[\\]{}()*+?.,\\\\^$|#\\s]/g" |> Maybe.withDefault Regex.never
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
    , receiverDebouncer =
        Debouncer.manual
            |> settleWhenQuietFor (Just <| fromSeconds 0.25)
            |> toDebouncer
    , selectedReceiver = Nothing
    , showReceivers = False
    , resultsHovered = False
    , key = key
    }
