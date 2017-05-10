module Views.SilenceList.Types exposing (SilenceListMsg(..), Model, initSilenceList)

import Utils.Types exposing (ApiData, ApiResponse(Loading))
import Silences.Types exposing (Silence)
import Utils.Types exposing (ApiData)
import Views.FilterBar.Types as FilterBar


type SilenceListMsg
    = SilenceDestroyed (ApiData String)
    | DestroySilence Silence
    | SilencesFetch (ApiData (List Silence))
    | FetchSilences
    | MsgForFilterBar FilterBar.Msg


type alias Model =
    { silences : ApiData (List Silence)
    , filterBar : FilterBar.Model
    }


initSilenceList : Model
initSilenceList =
    { silences = Loading
    , filterBar = FilterBar.initFilterBar
    }
