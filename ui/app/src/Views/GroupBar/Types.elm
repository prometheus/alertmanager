module Views.GroupBar.Types exposing (Model, Msg(..), initGroupBar)

import Browser.Navigation exposing (Key)
import Set exposing (Set)


type alias Model =
    { list : Set String
    , fieldText : String
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
    | CustomGrouping Bool
    | Noop


initGroupBar : Key -> Model
initGroupBar key =
    { list = Set.empty
    , fieldText = ""
    , fields = []
    , matches = []
    , focused = False
    , resultsHovered = False
    , backspacePressed = False
    , maybeSelectedMatch = Nothing
    , key = key
    }
