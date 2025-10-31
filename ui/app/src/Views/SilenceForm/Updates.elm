port module Views.SilenceForm.Updates exposing (update)

import Alerts.Api
import Browser.Navigation as Navigation
import Silences.Api
import Task
import Time
import Types exposing (Msg(..))
import Utils.Date exposing (timeFromString)
import Utils.DateTimePicker.Types exposing (initFromStartAndEndTime)
import Utils.DateTimePicker.Updates as DateTimePickerUpdates
import Utils.Filter exposing (silencePreviewFilter)
import Utils.FormValidation exposing (initialField, stringNotEmpty, updateValue, validate)
import Utils.Types exposing (ApiData(..))
import Views.FilterBar.Types as FilterBar
import Views.FilterBar.Updates as FilterBar
import Views.SilenceForm.Types
    exposing
        ( Model
        , SilenceForm
        , SilenceFormFieldMsg(..)
        , SilenceFormMsg(..)
        , fromDateTimePicker
        , fromMatchersAndCommentAndTime
        , fromSilence
        , parseEndsAt
        , toSilence
        , validateForm
        , validateMatchers
        )


updateForm : SilenceFormFieldMsg -> SilenceForm -> SilenceForm
updateForm msg form =
    case msg of
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

        UpdateTimesFromPicker ->
            let
                ( startsAt, endsAt, duration ) =
                    case ( form.dateTimePicker.startTime, form.dateTimePicker.endTime ) of
                        ( Just start, Just end ) ->
                            ( validate timeFromString (initialField (Utils.Date.timeToString start))
                            , validate (parseEndsAt (Utils.Date.timeToString start)) (initialField (Utils.Date.timeToString end))
                            , initialField (Utils.Date.durationFormat (Utils.Date.timeDifference start end) |> Maybe.withDefault "")
                                |> validate Utils.Date.parseDuration
                            )

                        _ ->
                            ( form.startsAt, form.endsAt, form.duration )
            in
            { form
                | startsAt = startsAt
                , endsAt = endsAt
                , duration = duration
                , viewDateTimePicker = False
            }

        OpenDateTimePicker ->
            let
                startsAtTime =
                    case timeFromString form.startsAt.value of
                        Ok time ->
                            Just time

                        _ ->
                            form.dateTimePicker.startTime

                endsAtTime =
                    timeFromString form.endsAt.value |> Result.toMaybe
            in
            { form
                | viewDateTimePicker = True
                , dateTimePicker = initFromStartAndEndTime startsAtTime endsAtTime form.dateTimePicker.firstDayOfWeek
            }

        CloseDateTimePicker ->
            { form
                | viewDateTimePicker = False
            }


update : SilenceFormMsg -> Model -> String -> String -> ( Model, Cmd Msg )
update msg model basePath apiUrl =
    case msg of
        CreateSilence ->
            case toSilence model.filterBar model.form of
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
                        , filterBarValid = validateMatchers model.filterBar
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

        NewSilenceFromMatchersAndComment defaultCreator params ->
            ( model, Task.perform (NewSilenceFromMatchersAndCommentAndTime defaultCreator params.matchers params.comment >> MsgForSilenceForm) Time.now )

        NewSilenceFromMatchersAndCommentAndTime defaultCreator matchers comment time ->
            ( { form = fromMatchersAndCommentAndTime defaultCreator comment time model.firstDayOfWeek
              , alerts = Initial
              , activeAlertId = Nothing
              , silenceId = Initial
              , filterBar = FilterBar.initFilterBar matchers
              , filterBarValid = Utils.FormValidation.Initial
              , key = model.key
              , firstDayOfWeek = model.firstDayOfWeek
              }
            , Cmd.none
            )

        FetchSilence silenceId ->
            ( model, Silences.Api.getSilence apiUrl silenceId (SilenceFetch >> MsgForSilenceForm) )

        SilenceFetch (Success silence) ->
            ( { form = fromSilence silence model.firstDayOfWeek
              , filterBar = FilterBar.initFilterBar (List.map Utils.Filter.fromApiMatcher silence.matchers)
              , filterBarValid = Utils.FormValidation.Initial
              , silenceId = model.silenceId
              , alerts = Initial
              , activeAlertId = Nothing
              , key = model.key
              , firstDayOfWeek = model.firstDayOfWeek
              }
            , Task.perform identity (Task.succeed (MsgForSilenceForm PreviewSilence))
            )

        SilenceFetch _ ->
            ( model, Cmd.none )

        PreviewSilence ->
            case toSilence model.filterBar model.form of
                Just silence ->
                    ( { model | alerts = Loading }
                    , Alerts.Api.fetchAlerts
                        apiUrl
                        (silencePreviewFilter silence.matchers)
                        |> Cmd.map (AlertGroupsPreview >> MsgForSilenceForm)
                    )

                Nothing ->
                    ( { model
                        | alerts = Failure "Can not display affected Alerts, Silence is not yet valid."
                        , form = validateForm model.form
                        , filterBarValid = validateMatchers model.filterBar
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
            ( { model
                | form = updateForm fieldMsg model.form
                , alerts = Initial
                , silenceId = Initial
              }
            , Cmd.none
            )

        UpdateDateTimePicker subMsg ->
            let
                newPicker =
                    DateTimePickerUpdates.update subMsg model.form.dateTimePicker
            in
            ( { model
                | form = fromDateTimePicker model.form newPicker
              }
            , Cmd.none
            )

        MsgForFilterBar subMsg ->
            let
                ( newFilterBar, _, subCmd ) =
                    FilterBar.update subMsg model.filterBar
            in
            ( { model | filterBar = newFilterBar, filterBarValid = Utils.FormValidation.Initial }
            , Cmd.map (MsgForFilterBar >> MsgForSilenceForm) subCmd
            )

        UpdateFirstDayOfWeek firstDayOfWeek ->
            ( { model
                | firstDayOfWeek = firstDayOfWeek
              }
            , Cmd.none
            )


port persistDefaultCreator : String -> Cmd msg
