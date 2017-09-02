module Views.SilenceView.Updates exposing (update)

import Views.SilenceView.Types exposing (Model, SilenceViewMsg(..))
import Silences.Api as Api exposing (getSilence)
import Alerts.Api
import Utils.List
import Utils.Types exposing (ApiData(..))
import Utils.Filter exposing (nullFilter)
import Navigation


update : SilenceViewMsg -> Model -> String -> String -> ( Model, Cmd SilenceViewMsg )
update msg model basePath apiUrl =
    case msg of
        DestroySilence silence ->
            -- TODO: "Deleted id: ID" growl
            -- TODO: Check why POST isn't there but is accepted
            ( model
            , Api.destroy apiUrl silence (always RedirectSilences)
            )

        RedirectSilences ->
            ( model
            , Navigation.newUrl (basePath ++ "#/silences")
            )

        FetchSilence id ->
            ( model, getSilence apiUrl id SilenceFetched )

        AlertGroupsPreview alerts ->
            ( { model | alerts = alerts }
            , Cmd.none
            )

        SilenceFetched (Success silence) ->
            ( { model
                | silence = Success silence
                , alerts = Loading
              }
            , Alerts.Api.fetchAlerts
                apiUrl
                ({ nullFilter | text = Just (Utils.List.mjoin silence.matchers), showSilenced = Just True })
                |> Cmd.map AlertGroupsPreview
            )

        SilenceFetched silence ->
            ( { model | silence = silence, alerts = Initial }, Cmd.none )

        InitSilenceView silenceId ->
            ( model, getSilence apiUrl silenceId SilenceFetched )
