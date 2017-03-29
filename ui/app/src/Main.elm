module Main exposing (..)

import Navigation
import Task
import Time
import Parsing
import Views
import Types
    exposing
        ( Route(..)
        , Msg
            ( NavigateToSilenceList
            , NavigateToSilence
            , NavigateToSilenceFormEdit
            , NavigateToSilenceFormNew
            , NavigateToAlerts
            , NavigateToNotFound
            , NavigateToStatus
            , UpdateCurrentTime
            , RedirectAlerts
            )
        , Model
        )
import Utils.Types exposing (..)
import Views.SilenceList.Updates
import Views.SilenceForm.Types exposing (initSilenceForm)
import Views.Status.Types exposing (StatusModel, initStatusModel)
import Updates exposing (update)


main : Program Never Model Msg
main =
    Navigation.program urlUpdate
        { init = init
        , update = update
        , view = Views.view
        , subscriptions = subscriptions
        }


init : Navigation.Location -> ( Model, Cmd Msg )
init location =
    let
        route =
            Parsing.urlParser location

        filter =
            case route of
                AlertsRoute filter ->
                    filter

                SilenceListRoute filter ->
                    filter

                _ ->
                    nullFilter

        ( model, msg ) =
            update (urlUpdate location) (Model Loading Loading initSilenceForm Loading route filter 0 initStatusModel)
    in
        model ! [ msg, Task.perform UpdateCurrentTime Time.now ]


urlUpdate : Navigation.Location -> Msg
urlUpdate location =
    let
        route =
            Parsing.urlParser location
    in
        case route of
            SilenceListRoute maybeFilter ->
                NavigateToSilenceList maybeFilter

            SilenceRoute silenceId ->
                NavigateToSilence silenceId

            SilenceFormEditRoute silenceId ->
                NavigateToSilenceFormEdit silenceId

            SilenceFormNewRoute keep ->
                NavigateToSilenceFormNew keep

            AlertsRoute filter ->
                NavigateToAlerts filter

            StatusRoute ->
                NavigateToStatus

            TopLevelRoute ->
                RedirectAlerts

            NotFoundRoute ->
                NavigateToNotFound



-- SUBSCRIPTIONS
-- TODO: Poll API for changes.


subscriptions : Model -> Sub Msg
subscriptions model =
    Sub.none
