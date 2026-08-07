"use strict";(self.webpackChunkfrontend=self.webpackChunkfrontend||[]).push([["3226"],{12433(e,t,n){n.d(t,{A:()=>i});var a=n(90290);let i=(0,a.pM)({name:"Add",render:()=>(0,a.h)("svg",{width:"512",height:"512",viewBox:"0 0 512 512",fill:"none",xmlns:"http://www.w3.org/2000/svg"},(0,a.h)("path",{d:"M256 112V400M400 256H112",stroke:"currentColor","stroke-width":"32","stroke-linecap":"round","stroke-linejoin":"round"}))})},53482(e,t,n){n.d(t,{g:()=>i});let a={tiny:"mini",small:"tiny",medium:"small",large:"medium",huge:"large"};function i(e){let t=a[e];if(void 0===t)throw Error(`${e} has no smaller size.`);return t}},14957(e,t,n){n.d(t,{$:()=>a});function a(e,t="default",n=[]){let i=e.$slots[t];return void 0===i?n:i()}},21683(e,t,n){let a;n.d(t,{A:()=>W});var i=n(5562),r=n(90290),l=n(98250),o=n(12433),s=n(49359),c=n(50922),d=n(53042),u=n(83370),h=n(4019),p=n(53482),v=n(16680),b=n(27546),g=n(45449),f=n(44041),m=n(79623),y=n(75454),w=n(69598),x=n(14957),k=n(49284);let A={name:"Space",self:function(){return k.A}};var C=n(91900);let B=Object.assign(Object.assign({},s.A.props),{align:String,justify:{type:String,default:"start"},inline:Boolean,vertical:Boolean,reverse:Boolean,size:{type:[String,Number,Array],default:"medium"},wrapItem:{type:Boolean,default:!0},itemClass:String,itemStyle:[String,Object],wrap:{type:Boolean,default:!0},internalUseGap:{type:Boolean,default:void 0}}),$=(0,r.pM)({name:"Space",props:B,setup(e){let{mergedClsPrefixRef:t,mergedRtlRef:n}=(0,c.Ay)(e),i=(0,s.A)("Space","-space",void 0,A,e,t),l=(0,m.I)("Space",n,t);return{useGap:function(){if(!C.B)return!0;if(void 0===a){let e=document.createElement("div");e.style.display="flex",e.style.flexDirection="column",e.style.rowGap="1px",e.appendChild(document.createElement("div")),e.appendChild(document.createElement("div")),document.body.appendChild(e);let t=1===e.scrollHeight;return document.body.removeChild(e),a=t}return a}(),rtlEnabled:l,mergedClsPrefix:t,margin:(0,r.EW)(()=>{let{size:t}=e;if(Array.isArray(t))return{horizontal:t[0],vertical:t[1]};if("number"==typeof t)return{horizontal:t,vertical:t};let{self:{[(0,y.cF)("gap",t)]:n}}=i.value,{row:a,col:r}=(0,f.t8)(n);return{horizontal:(0,f.eV)(r),vertical:(0,f.eV)(a)}})}},render(){let{vertical:e,reverse:t,align:n,inline:a,justify:i,itemClass:l,itemStyle:o,margin:s,wrap:c,mergedClsPrefix:d,rtlEnabled:u,useGap:h,wrapItem:p,internalUseGap:v}=this,b=(0,w.B)((0,x.$)(this),!1);if(!b.length)return null;let g=`${s.horizontal}px`,f=`${s.horizontal/2}px`,m=`${s.vertical}px`,y=`${s.vertical/2}px`,k=b.length-1,A=i.startsWith("space-");return(0,r.h)("div",{role:"none",class:[`${d}-space`,u&&`${d}-space--rtl`],style:{display:a?"inline-flex":"flex",flexDirection:e&&!t?"column":e&&t?"column-reverse":!e&&t?"row-reverse":"row",justifyContent:["start","end"].includes(i)?`flex-${i}`:i,flexWrap:!c||e?"nowrap":"wrap",marginTop:h||e?"":`-${y}`,marginBottom:h||e?"":`-${y}`,alignItems:n,gap:h?`${s.vertical}px ${s.horizontal}px`:""}},!p&&(h||v)?b:b.map((t,n)=>t.type===r.Mw?t:(0,r.h)("div",{role:"none",class:l,style:[o,{maxWidth:"100%"},h?"":e?{marginBottom:n!==k?m:""}:u?{marginLeft:A?"space-between"===i&&n===k?"":f:n!==k?g:"",marginRight:A?"space-between"===i&&0===n?"":f:"",paddingTop:y,paddingBottom:y}:{marginRight:A?"space-between"===i&&n===k?"":f:n!==k?g:"",marginLeft:A?"space-between"===i&&0===n?"":f:"",paddingTop:y,paddingBottom:y}]},t)))}});var S=n(89083),E=n(85602),V=n(28880),z=n(59662),F=n(93722),R=n(61470);let O=(0,s.a)({name:"DynamicTags",common:V.A,peers:{Input:F.A,Button:z.A,Tag:R.A,Space:A},self:()=>({inputWidth:"64px"})}),j=(0,y.cB)("dynamic-tags",[(0,y.cB)("input",{minWidth:"var(--n-input-width)"})]),_=Object.assign(Object.assign(Object.assign({},s.A.props),E.A),{size:{type:String,default:"medium"},closable:{type:Boolean,default:!0},defaultValue:{type:Array,default:()=>[]},value:Array,inputClass:String,inputStyle:[String,Object],inputProps:Object,max:Number,tagClass:String,tagStyle:[String,Object],renderTag:Function,onCreate:{type:Function,default:e=>e},"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],onChange:[Function,Array]}),W=(0,r.pM)({name:"DynamicTags",props:_,slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=(0,c.Ay)(e),{localeRef:a}=(0,d.A)("DynamicTags"),l=(0,u.A)(e),{mergedDisabledRef:o}=l,b=(0,r.KR)(""),g=(0,r.KR)(!1),f=(0,r.KR)(!0),m=(0,r.KR)(null),y=(0,s.A)("DynamicTags","-dynamic-tags",j,O,e,t),w=(0,r.KR)(e.defaultValue),x=(0,r.lW)(e,"value"),k=(0,i.A)(x,w),A=(0,r.EW)(()=>a.value.add),C=(0,r.EW)(()=>(0,p.g)(e.size)),B=(0,r.EW)(()=>o.value||!!e.max&&k.value.length>=e.max);function $(t){let{onChange:n,"onUpdate:value":a,onUpdateValue:i}=e,{nTriggerFormInput:r,nTriggerFormChange:o}=l;n&&(0,v.T)(n,t),i&&(0,v.T)(i,t),a&&(0,v.T)(a,t),w.value=t,r(),o()}function S(t){let n=null!=t?t:b.value;if(n){let t=k.value.slice(0);t.push(e.onCreate(n)),$(t)}g.value=!1,f.value=!0,b.value=""}let E=(0,r.EW)(()=>{let{self:{inputWidth:e}}=y.value;return{"--n-input-width":e}}),V=n?(0,h.R)("dynamic-tags",void 0,E,e):void 0;return{mergedClsPrefix:t,inputInstRef:m,localizedAdd:A,inputSize:C,inputValue:b,showInput:g,inputForceFocused:f,mergedValue:k,mergedDisabled:o,triggerDisabled:B,handleInputKeyDown:function(e){"Enter"===e.key&&S()},handleAddClick:function(){g.value=!0,(0,r.dY)(()=>{var e;null==(e=m.value)||e.focus(),f.value=!1})},handleInputBlur:function(){S()},handleCloseClick:function(e){let t=k.value.slice(0);t.splice(e,1),$(t)},handleInputConfirm:S,mergedTheme:y,cssVars:n?void 0:E,themeClass:null==V?void 0:V.themeClass,onRender:null==V?void 0:V.onRender}},render(){let{mergedTheme:e,cssVars:t,mergedClsPrefix:n,onRender:a,renderTag:i}=this;return null==a||a(),(0,r.h)($,{class:[`${n}-dynamic-tags`,this.themeClass],size:"small",style:t,theme:e.peers.Space,themeOverrides:e.peerOverrides.Space,itemStyle:"display: flex;"},{default:()=>{let{mergedTheme:e,tagClass:t,tagStyle:a,type:s,round:c,size:d,color:u,closable:h,mergedDisabled:p,showInput:v,inputValue:f,inputClass:m,inputStyle:y,inputSize:w,inputForceFocused:x,triggerDisabled:k,handleInputKeyDown:A,handleInputBlur:C,handleAddClick:B,handleCloseClick:$,handleInputConfirm:E,$slots:V}=this;return this.mergedValue.map((n,l)=>i?i(n,l):(0,r.h)(S.Ay,{key:l,theme:e.peers.Tag,themeOverrides:e.peerOverrides.Tag,class:t,style:a,type:s,round:c,size:d,color:u,closable:h,disabled:p,onClose:()=>{$(l)}},{default:()=>"string"==typeof n?n:n.label})).concat(v?V.input?V.input({submit:E,deactivate:C}):(0,r.h)(g.A,Object.assign({placeholder:"",size:w,style:y,class:m,autosize:!0},this.inputProps,{ref:"inputInstRef",value:f,onUpdateValue:e=>{this.inputValue=e},theme:e.peers.Input,themeOverrides:e.peerOverrides.Input,onKeydown:A,onBlur:C,internalForceFocus:x})):V.trigger?V.trigger({activate:B,disabled:k}):(0,r.h)(b.Ay,{dashed:!0,disabled:k,theme:e.peers.Button,themeOverrides:e.peerOverrides.Button,size:w,onClick:B},{icon:()=>(0,r.h)(l.A,{clsPrefix:n},{default:()=>(0,r.h)(o.A,null)})}))}})}})},45679(e,t,n){let a;n.d(t,{A:()=>A});var i=n(44041),r=n(5562),l=n(90290),o=n(39819),s=n(73445),c=n(49359),d=n(50922),u=n(83370),h=n(4019),p=n(16680),v=n(75454),b=n(49521),g=n(3755),f=n(28880),m=n(98090);let y={name:"Switch",common:f.A,self:function(e){let{primaryColor:t,opacityDisabled:n,borderRadius:a,textColor3:i}=e;return Object.assign(Object.assign({},m.A),{iconColor:i,textColor:"white",loadingColor:t,opacityDisabled:n,railColor:"rgba(0, 0, 0, .14)",railColorActive:t,buttonBoxShadow:"0 1px 4px 0 rgba(0, 0, 0, 0.3), inset 0 0 1px 0 rgba(0, 0, 0, 0.05)",buttonColor:"#FFF",railBorderRadiusSmall:a,railBorderRadiusMedium:a,railBorderRadiusLarge:a,buttonBorderRadiusSmall:a,buttonBorderRadiusMedium:a,buttonBorderRadiusLarge:a,boxShadowFocus:`0 0 0 2px ${(0,g.QX)(t,{alpha:.2})}`})}};var w=n(58454);let x=(0,v.cB)("switch",`
 height: var(--n-height);
 min-width: var(--n-width);
 vertical-align: middle;
 user-select: none;
 -webkit-user-select: none;
 display: inline-flex;
 outline: none;
 justify-content: center;
 align-items: center;
`,[(0,v.cE)("children-placeholder",`
 height: var(--n-rail-height);
 display: flex;
 flex-direction: column;
 overflow: hidden;
 pointer-events: none;
 visibility: hidden;
 `),(0,v.cE)("rail-placeholder",`
 display: flex;
 flex-wrap: none;
 `),(0,v.cE)("button-placeholder",`
 width: calc(1.75 * var(--n-rail-height));
 height: var(--n-rail-height);
 `),(0,v.cB)("base-loading",`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 font-size: calc(var(--n-button-width) - 4px);
 color: var(--n-loading-color);
 transition: color .3s var(--n-bezier);
 `,[(0,w.N)({left:"50%",top:"50%",originalTransform:"translateX(-50%) translateY(-50%)"})]),(0,v.cE)("checked, unchecked",`
 transition: color .3s var(--n-bezier);
 color: var(--n-text-color);
 box-sizing: border-box;
 position: absolute;
 white-space: nowrap;
 top: 0;
 bottom: 0;
 display: flex;
 align-items: center;
 line-height: 1;
 `),(0,v.cE)("checked",`
 right: 0;
 padding-right: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),(0,v.cE)("unchecked",`
 left: 0;
 justify-content: flex-end;
 padding-left: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),(0,v.c)("&:focus",[(0,v.cE)("rail",`
 box-shadow: var(--n-box-shadow-focus);
 `)]),(0,v.cM)("round",[(0,v.cE)("rail","border-radius: calc(var(--n-rail-height) / 2);",[(0,v.cE)("button","border-radius: calc(var(--n-button-height) / 2);")])]),(0,v.C5)("disabled",[(0,v.C5)("icon",[(0,v.cM)("rubber-band",[(0,v.cM)("pressed",[(0,v.cE)("rail",[(0,v.cE)("button","max-width: var(--n-button-width-pressed);")])]),(0,v.cE)("rail",[(0,v.c)("&:active",[(0,v.cE)("button","max-width: var(--n-button-width-pressed);")])]),(0,v.cM)("active",[(0,v.cM)("pressed",[(0,v.cE)("rail",[(0,v.cE)("button","left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));")])]),(0,v.cE)("rail",[(0,v.c)("&:active",[(0,v.cE)("button","left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));")])])])])])]),(0,v.cM)("active",[(0,v.cE)("rail",[(0,v.cE)("button","left: calc(100% - var(--n-button-width) - var(--n-offset))")])]),(0,v.cE)("rail",`
 overflow: hidden;
 height: var(--n-rail-height);
 min-width: var(--n-rail-width);
 border-radius: var(--n-rail-border-radius);
 cursor: pointer;
 position: relative;
 transition:
 opacity .3s var(--n-bezier),
 background .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 background-color: var(--n-rail-color);
 `,[(0,v.cE)("button-icon",`
 color: var(--n-icon-color);
 transition: color .3s var(--n-bezier);
 font-size: calc(var(--n-button-height) - 4px);
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 display: flex;
 justify-content: center;
 align-items: center;
 line-height: 1;
 `,[(0,w.N)()]),(0,v.cE)("button",`
 align-items: center; 
 top: var(--n-offset);
 left: var(--n-offset);
 height: var(--n-button-height);
 width: var(--n-button-width-pressed);
 max-width: var(--n-button-width);
 border-radius: var(--n-button-border-radius);
 background-color: var(--n-button-color);
 box-shadow: var(--n-button-box-shadow);
 box-sizing: border-box;
 cursor: inherit;
 content: "";
 position: absolute;
 transition:
 background-color .3s var(--n-bezier),
 left .3s var(--n-bezier),
 opacity .3s var(--n-bezier),
 max-width .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 `)]),(0,v.cM)("active",[(0,v.cE)("rail","background-color: var(--n-rail-color-active);")]),(0,v.cM)("loading",[(0,v.cE)("rail",`
 cursor: wait;
 `)]),(0,v.cM)("disabled",[(0,v.cE)("rail",`
 cursor: not-allowed;
 opacity: .5;
 `)])]),k=Object.assign(Object.assign({},c.A.props),{size:{type:String,default:"medium"},value:{type:[String,Number,Boolean],default:void 0},loading:Boolean,defaultValue:{type:[String,Number,Boolean],default:!1},disabled:{type:Boolean,default:void 0},round:{type:Boolean,default:!0},"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],checkedValue:{type:[String,Number,Boolean],default:!0},uncheckedValue:{type:[String,Number,Boolean],default:!1},railStyle:Function,rubberBand:{type:Boolean,default:!0},onChange:[Function,Array]}),A=(0,l.pM)({name:"Switch",props:k,slots:Object,setup(e){void 0===a&&(a="undefined"==typeof CSS||void 0!==CSS.supports&&CSS.supports("width","max(1px)"));let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=(0,d.Ay)(e),o=(0,c.A)("Switch","-switch",x,y,e,t),s=(0,u.A)(e),{mergedSizeRef:b,mergedDisabledRef:g}=s,f=(0,l.KR)(e.defaultValue),m=(0,l.lW)(e,"value"),w=(0,r.A)(m,f),k=(0,l.EW)(()=>w.value===e.checkedValue),A=(0,l.KR)(!1),C=(0,l.KR)(!1),B=(0,l.EW)(()=>{let{railStyle:t}=e;if(t)return t({focused:C.value,checked:k.value})});function $(t){let{"onUpdate:value":n,onChange:a,onUpdateValue:i}=e,{nTriggerFormInput:r,nTriggerFormChange:l}=s;n&&(0,p.T)(n,t),i&&(0,p.T)(i,t),a&&(0,p.T)(a,t),f.value=t,r(),l()}let S=(0,l.EW)(()=>{let e,t,n,{value:r}=b,{self:{opacityDisabled:l,railColor:s,railColorActive:c,buttonBoxShadow:d,buttonColor:u,boxShadowFocus:h,loadingColor:p,textColor:g,iconColor:f,[(0,v.cF)("buttonHeight",r)]:m,[(0,v.cF)("buttonWidth",r)]:y,[(0,v.cF)("buttonWidthPressed",r)]:w,[(0,v.cF)("railHeight",r)]:x,[(0,v.cF)("railWidth",r)]:k,[(0,v.cF)("railBorderRadius",r)]:A,[(0,v.cF)("buttonBorderRadius",r)]:C},common:{cubicBezierEaseInOut:B}}=o.value;return a?(e=`calc((${x} - ${m}) / 2)`,t=`max(${x}, ${m})`,n=`max(${k}, calc(${k} + ${m} - ${x}))`):(e=(0,i.Cw)(((0,i.eV)(x)-(0,i.eV)(m))/2),t=(0,i.Cw)(Math.max((0,i.eV)(x),(0,i.eV)(m))),n=(0,i.eV)(x)>(0,i.eV)(m)?k:(0,i.Cw)((0,i.eV)(k)+(0,i.eV)(m)-(0,i.eV)(x))),{"--n-bezier":B,"--n-button-border-radius":C,"--n-button-box-shadow":d,"--n-button-color":u,"--n-button-width":y,"--n-button-width-pressed":w,"--n-button-height":m,"--n-height":t,"--n-offset":e,"--n-opacity-disabled":l,"--n-rail-border-radius":A,"--n-rail-color":s,"--n-rail-color-active":c,"--n-rail-height":x,"--n-rail-width":k,"--n-width":n,"--n-box-shadow-focus":h,"--n-loading-color":p,"--n-text-color":g,"--n-icon-color":f}}),E=n?(0,h.R)("switch",(0,l.EW)(()=>b.value[0]),S,e):void 0;return{handleClick:function(){e.loading||g.value||(w.value!==e.checkedValue?$(e.checkedValue):$(e.uncheckedValue))},handleBlur:function(){C.value=!1,function(){let{nTriggerFormBlur:e}=s;e()}(),A.value=!1},handleFocus:function(){C.value=!0,function(){let{nTriggerFormFocus:e}=s;e()}()},handleKeyup:function(t){e.loading||g.value||" "===t.key&&(w.value!==e.checkedValue?$(e.checkedValue):$(e.uncheckedValue),A.value=!1)},handleKeydown:function(t){e.loading||g.value||" "===t.key&&(t.preventDefault(),A.value=!0)},mergedRailStyle:B,pressed:A,mergedClsPrefix:t,mergedValue:w,checked:k,mergedDisabled:g,cssVars:n?void 0:S,themeClass:null==E?void 0:E.themeClass,onRender:null==E?void 0:E.onRender}},render(){let{mergedClsPrefix:e,mergedDisabled:t,checked:n,mergedRailStyle:a,onRender:i,$slots:r}=this;null==i||i();let{checked:c,unchecked:d,icon:u,"checked-icon":h,"unchecked-icon":p}=r,v=!((0,b.yr)(u)&&(0,b.yr)(h)&&(0,b.yr)(p));return(0,l.h)("div",{role:"switch","aria-checked":n,class:[`${e}-switch`,this.themeClass,v&&`${e}-switch--icon`,n&&`${e}-switch--active`,t&&`${e}-switch--disabled`,this.round&&`${e}-switch--round`,this.loading&&`${e}-switch--loading`,this.pressed&&`${e}-switch--pressed`,this.rubberBand&&`${e}-switch--rubber-band`],tabindex:this.mergedDisabled?void 0:0,style:this.cssVars,onClick:this.handleClick,onFocus:this.handleFocus,onBlur:this.handleBlur,onKeyup:this.handleKeyup,onKeydown:this.handleKeydown},(0,l.h)("div",{class:`${e}-switch__rail`,"aria-hidden":"true",style:a},(0,b.iQ)(c,t=>(0,b.iQ)(d,n=>t||n?(0,l.h)("div",{"aria-hidden":!0,class:`${e}-switch__children-placeholder`},(0,l.h)("div",{class:`${e}-switch__rail-placeholder`},(0,l.h)("div",{class:`${e}-switch__button-placeholder`}),t),(0,l.h)("div",{class:`${e}-switch__rail-placeholder`},(0,l.h)("div",{class:`${e}-switch__button-placeholder`}),n)):null)),(0,l.h)("div",{class:`${e}-switch__button`},(0,b.iQ)(u,t=>(0,b.iQ)(h,n=>(0,b.iQ)(p,a=>(0,l.h)(o.A,null,{default:()=>this.loading?(0,l.h)(s.A,{key:"loading",clsPrefix:e,strokeWidth:20}):this.checked&&(n||t)?(0,l.h)("div",{class:`${e}-switch__button-icon`,key:n?"checked-icon":"icon"},n||t):!this.checked&&(a||t)?(0,l.h)("div",{class:`${e}-switch__button-icon`,key:a?"unchecked-icon":"icon"},a||t):null})))),(0,b.iQ)(c,t=>t&&(0,l.h)("div",{key:"checked",class:`${e}-switch__checked`},t)),(0,b.iQ)(d,t=>t&&(0,l.h)("div",{key:"unchecked",class:`${e}-switch__unchecked`},t)))))}})}}]);