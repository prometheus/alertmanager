module Views.SilenceList.Updates exposing (..)

import Silences.Api as Api
import Views.SilenceList.Types exposing (SilenceListMsg(..), Model)
import Silences.Types exposing (Silence, nullSilence, nullMatcher)
import Navigation
import Task
import Utils.Types as Types exposing (ApiData, ApiResponse(Failure, Loading, Success), Time, Matchers)
import Time
import Types exposing (Msg(UpdateCurrentTime, MsgForSilenceList), Route(SilenceListRoute))
import Utils.Filter exposing (Filter, generateQueryString)
import Views.FilterBar.Updates as FilterBar


update : SilenceListMsg -> Model -> ApiData Silence -> Filter -> ( Model, ApiData Silence, Cmd Types.Msg )
update msg model silence filter =
    case msg of
        SilencesFetch sils ->
            ( { model | silences = sils }, silence, Task.perform UpdateCurrentTime Time.now )

        FetchSilences ->
            ( { model
                | filterBar = FilterBar.setMatchers filter model.filterBar
                , silences = Loading
              }
            , silence
            , Api.getSilences filter (SilencesFetch >> MsgForSilenceList)
            )

        DestroySilence silence ->
            ( model, Loading, Api.destroy silence (SilenceDestroyed >> MsgForSilenceList) )

        SilenceDestroyed statusCode ->
            -- TODO: "Deleted id: ID" growl
            -- TODO: Check why POST isn't there but is accepted
            ( model, Loading, Navigation.newUrl "/#/silences" )

        MsgForFilterBar msg ->
            let
                ( filterBar, cmd ) =
                    FilterBar.update "/#/silences" filter msg model.filterBar
            in
                ( { model | filterBar = filterBar }, silence, Cmd.map (MsgForFilterBar >> MsgForSilenceList) cmd )


urlUpdate : Maybe String -> ( SilenceListMsg, Filter )
urlUpdate maybeString =
    ( FetchSilences, updateFilter maybeString )


updateFilter : Maybe String -> Filter
updateFilter maybeFilter =
    { receiver = Nothing
    , showSilenced = Nothing
    , group = Nothing
    , text = maybeFilter
    }
