module Views.SilenceList.Types exposing (SilenceListMsg(..), Model, initSilenceList)

import Utils.Types exposing (ApiData(Initial))
import Silences.Types exposing (Silence, State(Active))
import Utils.Types exposing (ApiData)
import Views.FilterBar.Types as FilterBar


type SilenceListMsg
    = DestroySilence Silence
    | SilencesFetch (ApiData (List Silence))
    | FetchSilences
    | MsgForFilterBar FilterBar.Msg
    | SetTab State


type alias Model =
    { silences : ApiData (List Silence)
    , filterBar : FilterBar.Model
    , tab : State
    }


initSilenceList : Model
initSilenceList =
    { silences = Initial
    , filterBar = FilterBar.initFilterBar
    , tab = Active
    }
