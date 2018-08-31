module Updates exposing (update)

import Navigation
import String exposing (trim)
import Task
import Types
    exposing
        ( Model
        , Msg(..)
        , Route(..)
        )
import Views.AlertList.Types exposing (AlertListMsg(..))
import Views.AlertList.Updates
import Views.SilenceForm.Types exposing (SilenceFormMsg(..))
import Views.SilenceForm.Updates
import Views.SilenceList.Types exposing (SilenceListMsg(..))
import Views.SilenceList.Updates
import Views.SilenceView.Types exposing (SilenceViewMsg(..))
import Views.SilenceView.Updates
import Views.Status.Types exposing (StatusMsg(..))
import Views.Status.Updates


update : Msg -> Model -> ( Model, Cmd Msg )
update msg ({ basePath, apiUrl } as model) =
    case msg of
        NavigateToAlerts filter ->
            let
                ( alertList, cmd ) =
                    Views.AlertList.Updates.update FetchAlerts model.alertList filter apiUrl basePath
            in
            ( { model | alertList = alertList, route = AlertsRoute filter, filter = filter }, cmd )

        NavigateToSilenceList filter ->
            let
                ( silenceList, cmd ) =
                    Views.SilenceList.Updates.update FetchSilences model.silenceList filter basePath apiUrl
            in
            ( { model | silenceList = silenceList, route = SilenceListRoute filter, filter = filter }
            , Cmd.map MsgForSilenceList cmd
            )

        NavigateToStatus ->
            ( { model | route = StatusRoute }, Task.perform identity (Task.succeed <| MsgForStatus InitStatusView) )

        NavigateToSilenceView silenceId ->
            let
                ( silenceView, cmd ) =
                    Views.SilenceView.Updates.update (InitSilenceView silenceId) model.silenceView apiUrl
            in
            ( { model | route = SilenceViewRoute silenceId, silenceView = silenceView }
            , Cmd.map MsgForSilenceView cmd
            )

        NavigateToSilenceFormNew matchers ->
            ( { model | route = SilenceFormNewRoute matchers }
            , Task.perform (NewSilenceFromMatchers model.defaultCreator >> MsgForSilenceForm) (Task.succeed matchers)
            )

        NavigateToSilenceFormEdit uuid ->
            ( { model | route = SilenceFormEditRoute uuid }, Task.perform identity (Task.succeed <| (FetchSilence uuid |> MsgForSilenceForm)) )

        NavigateToNotFound ->
            ( { model | route = NotFoundRoute }, Cmd.none )

        RedirectAlerts ->
            ( model, Navigation.newUrl (basePath ++ "#/alerts") )

        UpdateFilter text ->
            let
                t =
                    if trim text == "" then
                        Nothing

                    else
                        Just text

                prevFilter =
                    model.filter
            in
            ( { model | filter = { prevFilter | text = t } }, Cmd.none )

        Noop ->
            ( model, Cmd.none )

        MsgForStatus msg ->
            Views.Status.Updates.update msg model apiUrl

        MsgForAlertList msg ->
            let
                ( alertList, cmd ) =
                    Views.AlertList.Updates.update msg model.alertList model.filter apiUrl basePath
            in
            ( { model | alertList = alertList }, cmd )

        MsgForSilenceList msg ->
            let
                ( silenceList, cmd ) =
                    Views.SilenceList.Updates.update msg model.silenceList model.filter basePath apiUrl
            in
            ( { model | silenceList = silenceList }, Cmd.map MsgForSilenceList cmd )

        MsgForSilenceView msg ->
            let
                ( silenceView, cmd ) =
                    Views.SilenceView.Updates.update msg model.silenceView apiUrl
            in
            ( { model | silenceView = silenceView }, Cmd.map MsgForSilenceView cmd )

        MsgForSilenceForm msg ->
            let
                ( silenceForm, cmd ) =
                    Views.SilenceForm.Updates.update msg model.silenceForm basePath apiUrl
            in
            ( { model | silenceForm = silenceForm }, cmd )

        BootstrapCSSLoaded css ->
            ( { model | bootstrapCSS = css }, Cmd.none )

        FontAwesomeCSSLoaded css ->
            ( { model | fontAwesomeCSS = css }, Cmd.none )

        SetDefaultCreator name ->
            ( { model | defaultCreator = name }, Cmd.none )
