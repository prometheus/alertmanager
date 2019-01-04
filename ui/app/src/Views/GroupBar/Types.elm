module Views.GroupBar.Types exposing (Model, Msg(..), initGroupBar, updateDebouncer)

import Browser.Navigation exposing (Key)
import Debouncer.Messages as Debouncer exposing (Debouncer, fromSeconds, settleWhenQuietFor, toDebouncer)
import Set exposing (Set)


type alias Model =
    { list : Set String
    , fieldText : String
    , groupListDebouncer : Debouncer Msg
    , fields : List String
    , matches : List String
    , backspacePressed : Bool
    , focused : Bool
    , resultsHovered : Bool
    , maybeSelectedMatch : Maybe String
    , key : Key
    }


type Msg
    = AddField Bool String
    | DeleteField Bool String
    | Select (Maybe String)
    | PressingBackspace Bool
    | Focus Bool
    | ResultsHovered Bool
    | UpdateFieldText String
    | Noop
    | DebounceGroupList (Debouncer.Msg Msg)
    | FilterGroupList


updateDebouncer : Debouncer.UpdateConfig Msg Model
updateDebouncer =
    { mapMsg = DebounceGroupList
    , getDebouncer = .groupListDebouncer
    , setDebouncer = \debouncer model -> { model | groupListDebouncer = debouncer }
    }


initGroupBar : Key -> Model
initGroupBar key =
    { list = Set.empty
    , fieldText = ""
    , groupListDebouncer =
        Debouncer.manual
            |> settleWhenQuietFor (Just <| fromSeconds 0.25)
            |> toDebouncer
    , fields = []
    , matches = []
    , focused = False
    , resultsHovered = False
    , backspacePressed = False
    , maybeSelectedMatch = Nothing
    , key = key
    }
