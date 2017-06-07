module Views.SilenceForm.Updates exposing (update)

import Alerts.Api
import Silences.Api
import Task
import Time
import Navigation
import Utils.Date exposing (timeFromString)
import Utils.List
import Utils.Types exposing (ApiData(..))
import Utils.Filter exposing (nullFilter)
import Utils.FormValidation exposing (updateValue, validate, stringNotEmpty, fromResult)
import Views.SilenceForm.Types
    exposing
        ( Model
        , SilenceForm
        , SilenceFormMsg(..)
        , SilenceFormFieldMsg(..)
        , fromMatchersAndTime
        , fromSilence
        , validateForm
        , toSilence
        , emptyMatcher
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
                    case Result.map2 (-) endsAt startsAt of
                        Ok duration ->
                            Utils.Date.durationFormat duration

                        Err _ ->
                            form.duration.value
            in
                { form
                    | startsAt = updateValue time form.startsAt
                    , duration = updateValue durationValue form.duration
                }

        ValidateStartsAt ->
            { form
                | startsAt = validate Utils.Date.timeFromString form.startsAt
            }

        UpdateEndsAt time ->
            let
                endsAt =
                    Utils.Date.timeFromString time

                startsAt =
                    Utils.Date.timeFromString form.startsAt.value

                durationValue =
                    case Result.map2 (-) endsAt startsAt of
                        Ok duration ->
                            Utils.Date.durationFormat duration

                        Err _ ->
                            form.duration.value
            in
                { form
                    | endsAt = updateValue time form.endsAt
                    , duration = updateValue durationValue form.duration
                }

        ValidateEndsAt ->
            { form
                | endsAt = validate Utils.Date.timeFromString form.endsAt
            }

        UpdateDuration time ->
            let
                duration =
                    Utils.Date.parseDuration time

                startsAt =
                    Utils.Date.timeFromString form.startsAt.value

                endsAtValue =
                    case Result.map2 (+) startsAt duration of
                        Ok endsAt ->
                            Utils.Date.timeToString endsAt

                        Err _ ->
                            form.endsAt.value
            in
                { form
                    | endsAt = updateValue endsAtValue form.endsAt
                    , duration = updateValue time form.duration
                }

        ValidateDuration ->
            { form
                | duration = validate Utils.Date.parseDuration form.duration
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
                        (\matcher -> { matcher | value = validate stringNotEmpty matcher.value })
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


update : SilenceFormMsg -> Model -> String -> String -> ( Model, Cmd SilenceFormMsg )
update msg model basePath apiUrl =
    case msg of
        CreateSilence ->
            case toSilence model.form of
                Just silence ->
                    ( { model | silenceId = Loading }
                    , Silences.Api.create apiUrl silence |> Cmd.map SilenceCreate
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
                            Navigation.newUrl (basePath ++ "#/silences/" ++ id)

                        _ ->
                            Cmd.none
            in
                ( { model | silenceId = silenceId }, cmd )

        NewSilenceFromMatchers matchers ->
            ( model, Task.perform (NewSilenceFromMatchersAndTime matchers) Time.now )

        NewSilenceFromMatchersAndTime matchers time ->
            ( { form = fromMatchersAndTime matchers time
              , alerts = Initial
              , silenceId = Initial
              }
            , Cmd.none
            )

        FetchSilence silenceId ->
            ( model, Silences.Api.getSilence apiUrl silenceId SilenceFetch )

        SilenceFetch (Success silence) ->
            ( { model | form = fromSilence silence }
            , Task.perform identity (Task.succeed PreviewSilence)
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
                        |> Cmd.map AlertGroupsPreview
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

        UpdateField fieldMsg ->
            ( { form = updateForm fieldMsg model.form
              , alerts = Initial
              , silenceId = Initial
              }
            , Cmd.none
            )
