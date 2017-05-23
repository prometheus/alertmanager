module Main exposing (main)

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
import Utils.Filter exposing (nullFilter)
import Views.SilenceForm.Types exposing (initSilenceForm)
import Views.Status.Types exposing (StatusModel, initStatusModel)
import Views.AlertList.Types exposing (initAlertList)
import Views.SilenceList.Types exposing (initSilenceList)
import Updates exposing (update)
import Subscriptions exposing (subscriptions)


main : Program Never Model Msg
main =
    Navigation.program urlUpdate
        { init = init
        , update = update
        , view = Views.view
        , subscriptions = always Sub.none
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
            update (urlUpdate location) (Model initSilenceList Loading initSilenceForm initAlertList route filter 0 initStatusModel)
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
