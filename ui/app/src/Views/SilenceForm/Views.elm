module Views.SilenceForm.Views exposing (edit, new)

import Html exposing (Html, div, text, fieldset, legend, label, span, a)
import Html.Attributes exposing (class, href)
import Html.Events exposing (onClick)
import Silences.Types exposing (Silence)
import Types exposing (Msg(MsgForSilenceList, PreviewSilence, MsgForSilenceForm), Model)
import Utils.Types exposing (Matcher, ApiResponse(Success, Loading, Failure), ApiData)
import Utils.Views exposing (loading, error, checkbox)
import Views.Shared.SilencePreview
import Utils.Views exposing (formField, iconButtonMsg, textField, formInput)
import Views.SilenceForm.Types
    exposing
        ( SilenceFormMsg(UpdateStartsAt, UpdateCreatedBy, UpdateDuration, UpdateEndsAt, UpdateComment, AddMatcher, CreateSilence, DeleteMatcher, UpdateMatcherRegex, UpdateMatcherValue, UpdateMatcherName)
        )


edit : ApiData Silence -> Html Msg
edit silence =
    case silence of
        Success silence ->
            silenceForm "Edit" silence

        Loading ->
            loading

        Failure msg ->
            error msg


new : ApiData Silence -> Html Msg
new silence =
    case silence of
        Success silence ->
            silenceForm "New" silence

        Loading ->
            loading

        Failure msg ->
            error msg


silenceForm : String -> Silence -> Html Msg
silenceForm kind silence =
    -- TODO: Add field validations.
    let
        base =
            "/#/silence/"

        boundMatcherForm =
            matcherForm silence

        url =
            case kind of
                "New" ->
                    base ++ "new"

                "Edit" ->
                    base ++ (toString silence.id) ++ "/edit"

                _ ->
                    "/#/silences"
    in
        div []
            [ div [ class "pa4 black-80" ]
                [ fieldset [ class "ba b--transparent ph0 mh0" ]
                    [ legend [ class "ph0 mh0 fw6" ] [ text <| kind ++ " Silence" ]
                    , (formField "Start" silence.startsAt.s (UpdateStartsAt silence))
                    , div [ class "dib mb2 mr2 w-40" ] [ formField "End" silence.endsAt.s (UpdateEndsAt silence) ]
                    , div [ class "dib mb2 mr2 w-40" ] [ formField "Duration" silence.duration.s (UpdateDuration silence) ]
                    , div [ class "mt3" ]
                        [ label [ class "f6 b db mb2" ]
                            [ text "Matchers "
                            , span [ class "normal black-60" ] [ text "Alerts affected by this silence. Format: name=value" ]
                            ]
                        , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Name" ]
                        , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Value" ]
                        ]
                    , div [] (List.map boundMatcherForm silence.matchers)
                    , iconButtonMsg "blue" "fa-plus" (AddMatcher silence)
                    , formField "Creator" silence.createdBy (UpdateCreatedBy silence)
                    , textField "Comment" silence.comment (UpdateComment silence)
                    , div [ class "mt3" ]
                        [ a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", onClick (CreateSilence silence) ] [ text "Create" ]
                          -- Reset isn't working for "New" -- just updates time.
                        , a [ class "f6 link br2 ba ph3 pv2 mr2 dib dark-red", href url ] [ text "Reset" ]
                        ]
                    ]
                    |> (Html.map MsgForSilenceForm)
                , div [ class "mt3" ]
                    [ a [ class "f6 link br2 ba ph3 pv2 mr2 dib dark-green", onClick <| PreviewSilence silence ] [ text "Show Affected Alerts" ]
                    ]
                , Views.Shared.SilencePreview.view silence
                ]
            ]


matcherForm : Silence -> Matcher -> Html SilenceFormMsg
matcherForm silence matcher =
    div []
        [ formInput matcher.name (UpdateMatcherName silence matcher)
        , formInput matcher.value (UpdateMatcherValue silence matcher)
        , checkbox "Regex" matcher.isRegex (UpdateMatcherRegex silence matcher)
        , iconButtonMsg "dark-red" "fa-trash-o" (DeleteMatcher silence matcher)
        ]
