module Views.SilenceList.Updates exposing (..)

import Silences.Api as Api
import Views.SilenceList.Types exposing (SilenceListMsg(..))
import Silences.Types exposing (Silence, nullSilence, nullMatcher)
import Navigation
import Task
import Utils.Types exposing (ApiData, ApiResponse(..), Filter, Matchers)
import Utils.Types as Types exposing (ApiData, ApiResponse(Failure, Loading, Success), Time, Filter, Matchers)
import Time
import Types exposing (Msg(UpdateCurrentTime, MsgForSilenceList), Route(SilenceListRoute))
import Utils.Filter exposing (generateQueryString)


update : SilenceListMsg -> ApiData (List Silence) -> ApiData Silence -> Filter -> ( ApiData (List Silence), ApiData Silence, Cmd Types.Msg )
update msg silences silence filter =
    case msg of
        SilencesFetch sils ->
            ( sils, silence, Task.perform UpdateCurrentTime Time.now )

        FetchSilences ->
            ( silences, silence, Api.getSilences filter (SilencesFetch >> MsgForSilenceList) )

        DestroySilence silence ->
            ( silences, Loading, Api.destroy silence (SilenceDestroy >> MsgForSilenceList) )

        SilenceDestroy silence ->
            -- TODO: "Deleted id: ID" growl
            -- TODO: Add DELETE to accepted CORS methods in alertmanager
            -- TODO: Check why POST isn't there but is accepted
            ( silences, Loading, Navigation.newUrl "/#/silences" )

        FilterSilences ->
            ( silences, silence, Navigation.newUrl ("/#/silences" ++ generateQueryString filter) )


urlUpdate : Maybe String -> ( SilenceListMsg, Filter )
urlUpdate maybeString =
    ( FetchSilences, updateFilter maybeString )


updateFilter : Maybe String -> Filter
updateFilter maybeFilter =
    { receiver = Nothing
    , showSilenced = Nothing
    , text = maybeFilter
    }
