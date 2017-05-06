module Views.AutoComplete.Types exposing (Model, Msg(..), initAutoComplete)

import Set exposing (Set)


type alias Model =
    { list : Set String
    , fieldText : String
    , fields : List String
    }


type Msg
    = AddField Bool String
    | DeleteField Bool String
    | PressingBackspace Bool
    | UpdateFieldText String
    | Noop


initAutoComplete : Model
initAutoComplete =
    { list = Set.empty
    , fieldText = ""
    , fields = []
    }
