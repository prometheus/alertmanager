port module Views.SilenceForm.Updates exposing (update)

import Alerts.Api
import Browser.Navigation as Navigation
import Silences.Api
import Task
import Time
import Types exposing (Msg(..))
import Utils.Date exposing (timeFromString)
import Utils.Filter exposing (nullFilter)
import Utils.FormValidation exposing (fromResult, stringNotEmpty, updateValue, validate)
import Utils.List
import Utils.Types exposing (ApiData(..))
import Views.SilenceForm.Types
    exposing
        ( Model
        , SilenceForm
        , SilenceFormFieldMsg(..)
        , SilenceFormMsg(..)
        , emptyMatcher
        , fromMatchersAndTime
        , fromSilence
        , parseEndsAt
        , toSilence
        , validateForm
        )


updateForm : SilenceFormFieldMsg -> SilenceForm -> SilenceForm
updateForm msg form =
    case msg of
        AddMatcher ->
            { form | matchers = form.matchers ++ [ emptyMatcher ] }

        UpdateStartsAt time ->
            let
                startsAt =
                    Utils.Date.timeFromString time

                endsAt =
                    Utils.Date.timeFromString form.endsAt.value

                durationValue =
                    case Result.map2 Utils.Date.timeDifference startsAt endsAt of
                        Ok duration ->
                            case Utils.Date.durationFormat duration of
                                Just value ->
                                    value

                                Nothing ->
                                    form.duration.value

                        Err _ ->
                            form.duration.value
            in
            { form
                | startsAt = updateValue time form.startsAt
                , duration = updateValue durationValue form.duration
            }

        UpdateEndsAt time ->
            let
                endsAt =
                    Utils.Date.timeFromString time

                startsAt =
                    Utils.Date.timeFromString form.startsAt.value

                durationValue =
                    case Result.map2 Utils.Date.timeDifference startsAt endsAt of
                        Ok duration ->
                            case Utils.Date.durationFormat duration of
                                Just value ->
                                    value

                                Nothing ->
                                    form.duration.value

                        Err _ ->
                            form.duration.value
            in
            { form
                | endsAt = updateValue time form.endsAt
                , duration = updateValue durationValue form.duration
            }

        UpdateDuration time ->
            let
                duration =
                    Utils.Date.parseDuration time

                startsAt =
                    Utils.Date.timeFromString form.startsAt.value

                endsAtValue =
                    case Result.map2 Utils.Date.addDuration duration startsAt of
                        Ok endsAt ->
                            Utils.Date.timeToString endsAt

                        Err _ ->
                            form.endsAt.value
            in
            { form
                | endsAt = updateValue endsAtValue form.endsAt
                , duration = updateValue time form.duration
            }

        ValidateTime ->
            { form
                | startsAt = validate Utils.Date.timeFromString form.startsAt
                , endsAt = validate (parseEndsAt form.startsAt.value) form.endsAt
                , duration = validate Utils.Date.parseDuration form.duration
            }

        UpdateCreatedBy createdBy ->
            { form | createdBy = updateValue createdBy form.createdBy }

        ValidateCreatedBy ->
            { form | createdBy = validate stringNotEmpty form.createdBy }

        UpdateComment comment ->
            { form | comment = updateValue comment form.comment }

        ValidateComment ->
            { form | comment = validate stringNotEmpty form.comment }

        DeleteMatcher index ->
            { form | matchers = List.take index form.matchers ++ List.drop (index + 1) form.matchers }

        UpdateMatcherName index name ->
            let
                matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | name = updateValue name matcher.name })
                        form.matchers
            in
            { form | matchers = matchers }

        ValidateMatcherName index ->
            let
                matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | name = validate stringNotEmpty matcher.name })
                        form.matchers
            in
            { form | matchers = matchers }

        UpdateMatcherValue index value ->
            let
                matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | value = updateValue value matcher.value })
                        form.matchers
            in
            { form | matchers = matchers }

        ValidateMatcherValue index ->
            let
                matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | value = matcher.value })
                        form.matchers
            in
            { form | matchers = matchers }

        UpdateMatcherRegex index isRegex ->
            let
                matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | isRegex = isRegex })
                        form.matchers
            in
            { form | matchers = matchers }


update : SilenceFormMsg -> Model -> String -> String -> ( Model, Cmd Msg )
update msg model basePath apiUrl =
    case msg of
        CreateSilence ->
            case toSilence model.form of
                Just silence ->
                    ( { model | silenceId = Loading }
                    , Cmd.batch
                        [ Silences.Api.create apiUrl silence |> Cmd.map (SilenceCreate >> MsgForSilenceForm)
                        , persistDefaultCreator silence.createdBy
                        , Task.succeed silence.createdBy |> Task.perform SetDefaultCreator
                        ]
                    )

                Nothing ->
                    ( { model
                        | silenceId = Failure "Could not submit the form, Silence is not yet valid."
                        , form = validateForm model.form
                      }
                    , Cmd.none
                    )

        SilenceCreate silenceId ->
            let
                cmd =
                    case silenceId of
                        Success id ->
                            Navigation.pushUrl model.key (basePath ++ "#/silences/" ++ id)

                        _ ->
                            Cmd.none
            in
            ( { model | silenceId = silenceId }, cmd )

        NewSilenceFromMatchers defaultCreator matchers ->
            ( model, Task.perform (NewSilenceFromMatchersAndTime defaultCreator matchers >> MsgForSilenceForm) Time.now )

        NewSilenceFromMatchersAndTime defaultCreator matchers time ->
            ( { form = fromMatchersAndTime defaultCreator matchers time
              , alerts = Initial
              , activeAlertId = Nothing
              , silenceId = Initial
              , key = model.key
              }
            , Cmd.none
            )

        FetchSilence silenceId ->
            ( model, Silences.Api.getSilence apiUrl silenceId (SilenceFetch >> MsgForSilenceForm) )

        SilenceFetch (Success silence) ->
            ( { model | form = fromSilence silence }
            , Task.perform identity (Task.succeed (MsgForSilenceForm PreviewSilence))
            )

        SilenceFetch _ ->
            ( model, Cmd.none )

        PreviewSilence ->
            case toSilence model.form of
                Just silence ->
                    ( { model | alerts = Loading }
                    , Alerts.Api.fetchAlerts
                        apiUrl
                        { nullFilter | text = Just (Utils.List.mjoin silence.matchers) }
                        |> Cmd.map (AlertGroupsPreview >> MsgForSilenceForm)
                    )

                Nothing ->
                    ( { model
                        | alerts = Failure "Can not display affected Alerts, Silence is not yet valid."
                        , form = validateForm model.form
                      }
                    , Cmd.none
                    )

        AlertGroupsPreview alerts ->
            ( { model | alerts = alerts }
            , Cmd.none
            )

        SetActiveAlert maybeAlertId ->
            ( { model | activeAlertId = maybeAlertId }
            , Cmd.none
            )

        UpdateField fieldMsg ->
            ( { form = updateForm fieldMsg model.form
              , alerts = Initial
              , silenceId = Initial
              , key = model.key
              , activeAlertId = model.activeAlertId
              }
            , Cmd.none
            )


port persistDefaultCreator : String -> Cmd msg
